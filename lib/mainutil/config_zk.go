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
	"github.com/rs/zerolog/log"
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
	Auth           *zacJSON      `json:"auth,omitempty"`
}

type zacJSON struct {
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
		return nullBytes, nil
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

	if str == "" || str == nullString {
		return nil
	}

	err := strictUnmarshalJSON([]byte(str), zc)
	if err == nil {
		wantZero = false
		return nil
	}

	pieces := strings.Split(str, ";")

	if pieces[0] == "" {
		return nil
	}

	serverList := strings.Split(pieces[0], ",")
	zc.Servers = make([]string, 0, len(serverList))
	for _, server := range serverList {
		if server == "" {
			continue
		}
		if _, _, err := net.SplitHostPort(server); err != nil {
			server = net.JoinHostPort(server, "2181")
		}
		zc.Servers = append(zc.Servers, server)
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
			zc.Auth.Raw, err = tryB64DecodeString(item[9:])
			if err != nil {
				return err
			}

		case strings.HasPrefix(item, "username="):
			zc.Auth.Enabled = true
			if zc.Auth.Scheme == "" {
				zc.Auth.Scheme = "digest"
			}
			zc.Auth.Username = item[9:]

		case strings.HasPrefix(item, "password="):
			zc.Auth.Enabled = true
			zc.Auth.Password = item[9:]

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

	if bytes.Equal(raw, nullBytes) {
		return nil
	}

	var alt zcJSON
	err := strictUnmarshalJSON(raw, &alt)
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

	var altAuth *zacJSON
	if zc.Auth.Enabled {
		altAuth = &zacJSON{
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
		raw, err := tryB64DecodeString(alt.Auth.Raw)
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
	defer func() {
		log.Logger.Trace().
			Interface("result", out).
			Msg("ZKConfig parse result")
	}()

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

func tryB64DecodeString(str string) ([]byte, error) {
	if str == "" {
		return nil, nil
	}

	var firstErr error
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.RawURLEncoding,
	} {
		raw, err := enc.DecodeString(str)
		if err == nil {
			return raw, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return nil, fmt.Errorf("failed to decode base-64 string %q: %w", str, firstErr)
}
