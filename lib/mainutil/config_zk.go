package mainutil

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/go-zookeeper/zk"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

type ZKConfig struct {
	Enabled        bool
	Servers        []string
	SessionTimeout time.Duration
	Auth           ZKAuthConfig
}

type ZKAuthConfig struct {
	Enabled  bool
	Scheme   string
	Raw      []byte
	Username string
	Password string
}

type zcJSON struct {
	Servers        []string      `json:"servers"`
	SessionTimeout time.Duration `json:"sessionTimeout,omitempty"`
	Auth           *zcaJSON      `json:"auth,omitempty"`
}

type zcaJSON struct {
	Scheme   string `json:"scheme"`
	Raw      string `json:"raw,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

func (zc ZKConfig) AppendTo(out *strings.Builder) {
	if !zc.Enabled {
		return
	}
	for i, server := range zc.Servers {
		if i != 0 {
			out.WriteString(",")
		}
		out.WriteString(server)
	}
	if zc.SessionTimeout != 0 {
		out.WriteString(";sessionTimeout=")
		out.WriteString(zc.SessionTimeout.String())
	}
	if !zc.Auth.Enabled {
		return
	}
	if zc.Auth.Scheme != "" {
		out.WriteString(";authtype=")
		out.WriteString(zc.Auth.Scheme)
	}
	if zc.Auth.Raw != nil {
		out.WriteString(";authdata=")
		out.WriteString(base64.StdEncoding.EncodeToString(zc.Auth.Raw))
	}
	if zc.Auth.Username != "" {
		out.WriteString(";username=")
		out.WriteString(zc.Auth.Username)
	}
	if zc.Auth.Password != "" {
		out.WriteString(";password=")
		out.WriteString(zc.Auth.Password)
	}
}

func (zc ZKConfig) String() string {
	if !zc.Enabled {
		return ""
	}

	var buf strings.Builder
	buf.Grow(64)
	zc.AppendTo(&buf)
	return buf.String()
}

func (zc ZKConfig) MarshalJSON() ([]byte, error) {
	if !zc.Enabled {
		return constants.NullBytes, nil
	}
	return json.Marshal(zc.toAlt())
}

func (zc *ZKConfig) Parse(str string) error {
	wantZero := true
	defer func() {
		if wantZero {
			*zc = ZKConfig{}
		}
	}()

	if str == "" || str == constants.NullString {
		return nil
	}

	err := misc.StrictUnmarshalJSON([]byte(str), zc)
	if err == nil {
		wantZero = false
		return nil
	}

	pieces := strings.Split(str, ";")

	if pieces[0] == "" {
		return nil
	}

	serverListString, err := roxyutil.ExpandString(pieces[0])
	if err != nil {
		return err
	}

	serverList := strings.Split(serverListString, ",")
	zc.Servers = make([]string, 0, len(serverList))
	for _, server := range serverList {
		if server == "" {
			continue
		}
		host, port, err := misc.SplitHostPort(server, constants.PortZK)
		if err != nil {
			return err
		}
		zc.Servers = append(zc.Servers, net.JoinHostPort(host, port))
	}
	if len(zc.Servers) == 0 {
		return nil
	}

	for _, item := range pieces[1:] {
		switch {
		case strings.HasPrefix(item, "sessionTimeout="):
			zc.SessionTimeout, err = time.ParseDuration(item[15:])
			if err != nil {
				return err
			}

		case strings.HasPrefix(item, "authtype="):
			zc.Auth.Enabled = true
			zc.Auth.Scheme = item[9:]

		case strings.HasPrefix(item, "authdata="):
			zc.Auth.Enabled = true
			zc.Auth.Raw, err = misc.TryBase64DecodeString(item[9:])
			if err != nil {
				return err
			}

		case strings.HasPrefix(item, "username="):
			expanded, err := roxyutil.ExpandString(item[9:])
			if err != nil {
				return err
			}
			zc.Auth.Enabled = true
			if zc.Auth.Scheme == "" {
				zc.Auth.Scheme = "digest"
			}
			zc.Auth.Username = expanded

		case strings.HasPrefix(item, "password="):
			expanded, err := roxyutil.ExpandPassword(item[9:])
			if err != nil {
				return err
			}
			zc.Auth.Enabled = true
			zc.Auth.Password = expanded

		default:
			return fmt.Errorf("unknown option %q", item)
		}
	}

	zc.Enabled = true
	tmp, err := zc.postprocess()
	if err != nil {
		return err
	}

	*zc = tmp
	wantZero = false
	return nil
}

func (zc *ZKConfig) UnmarshalJSON(raw []byte) error {
	wantZero := true
	defer func() {
		if wantZero {
			*zc = ZKConfig{}
		}
	}()

	if bytes.Equal(raw, constants.NullBytes) {
		return nil
	}

	var alt zcJSON
	err := misc.StrictUnmarshalJSON(raw, &alt)
	if err != nil {
		return err
	}

	tmp1, err := alt.toStd()
	if err != nil {
		return err
	}

	tmp2, err := tmp1.postprocess()
	if err != nil {
		return err
	}

	*zc = tmp2
	wantZero = false
	return nil
}

func (zc ZKConfig) Connect(ctx context.Context) (*zk.Conn, error) {
	if !zc.Enabled {
		return nil, nil
	}

	sessTimeout := zc.SessionTimeout
	if sessTimeout == 0 {
		sessTimeout = 30 * time.Second
	}

	zkconn, _, err := zk.Connect(
		zc.Servers,
		sessTimeout,
		zk.WithLogger(ZKLoggerBridge{}))
	if err != nil {
		return nil, err
	}

	if zc.Auth.Enabled {
		var authRaw []byte
		if zc.Auth.Raw != nil {
			authRaw = zc.Auth.Raw
		} else {
			authRaw = []byte(zc.Auth.Username + ":" + zc.Auth.Password)
		}

		err = zkconn.AddAuth(zc.Auth.Scheme, authRaw)
		if err != nil {
			zkconn.Close()
			return nil, fmt.Errorf("failed to (*zk.Conn).AddAuth with scheme=%q: %w", zc.Auth.Scheme, err)
		}
	}

	return zkconn, nil
}

func (zc ZKConfig) toAlt() *zcJSON {
	if !zc.Enabled {
		return nil
	}

	var altAuth *zcaJSON
	if zc.Auth.Enabled {
		altAuth = &zcaJSON{
			Scheme:   zc.Auth.Scheme,
			Raw:      base64.StdEncoding.EncodeToString(zc.Auth.Raw),
			Username: zc.Auth.Username,
			Password: zc.Auth.Password,
		}
	}

	return &zcJSON{
		Servers:        zc.Servers,
		SessionTimeout: zc.SessionTimeout,
		Auth:           altAuth,
	}
}

func (alt *zcJSON) toStd() (ZKConfig, error) {
	if alt == nil {
		return ZKConfig{}, nil
	}

	var stdAuth ZKAuthConfig
	if alt.Auth != nil {
		raw, err := misc.TryBase64DecodeString(alt.Auth.Raw)
		if err != nil {
			return ZKConfig{}, err
		}

		stdAuth = ZKAuthConfig{
			Enabled:  true,
			Scheme:   alt.Auth.Scheme,
			Raw:      raw,
			Username: alt.Auth.Username,
			Password: alt.Auth.Password,
		}
	}

	return ZKConfig{
		Enabled:        true,
		Servers:        alt.Servers,
		SessionTimeout: alt.SessionTimeout,
		Auth:           stdAuth,
	}, nil
}

func (zc ZKConfig) postprocess() (out ZKConfig, err error) {
	var zero ZKConfig

	if !zc.Enabled {
		return zero, nil
	}

	if len(zc.Servers) == 0 {
		return zero, nil
	}

	for _, server := range zc.Servers {
		if server == "" {
			return zero, nil
		}
	}

	if !zc.Auth.Enabled {
		zc.Auth = ZKAuthConfig{}
	}

	if zc.Auth.Enabled {
		if zc.Auth.Raw == nil && zc.Auth.Username == "" {
			return zero, errors.New("must specify either \"Auth.Raw\" or \"Auth.Username\"")
		}
		if zc.Auth.Raw != nil && zc.Auth.Username != "" {
			return zero, errors.New("cannot specify both \"Auth.Raw\" and \"Auth.Username\"")
		}
		if zc.Auth.Scheme == "" && zc.Auth.Raw != nil {
			return zero, errors.New("must specify \"Auth.Scheme\"")
		}
		if zc.Auth.Scheme == "" {
			zc.Auth.Scheme = "digest"
		}
	}

	return zc, nil
}
