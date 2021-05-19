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

// ZKConfig represents the configuration for a *zk.Conn.
type ZKConfig struct {
	Enabled        bool
	Servers        []string
	SessionTimeout time.Duration
	Auth           ZKAuthConfig
}

// ZKAuthConfig represents the configuration for (*zk.Conn).AddAuth.
type ZKAuthConfig struct {
	Enabled  bool
	Scheme   string
	Raw      []byte
	Username string
	Password string
}

// ZKConfigJSON represents the JSON doppelgänger of an ZKConfig.
type ZKConfigJSON struct {
	Servers        []string          `json:"servers"`
	SessionTimeout time.Duration     `json:"sessionTimeout,omitempty"`
	Auth           *ZKAuthConfigJSON `json:"auth,omitempty"`
}

// ZKAuthConfigJSON represents the JSON doppelgänger of an ZKAuthConfig.
type ZKAuthConfigJSON struct {
	Scheme   string `json:"scheme"`
	Raw      string `json:"raw,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// AppendTo appends the string representation to the given Builder.
func (cfg ZKConfig) AppendTo(out *strings.Builder) {
	if !cfg.Enabled {
		return
	}
	for i, server := range cfg.Servers {
		if i != 0 {
			out.WriteString(",")
		}
		out.WriteString(server)
	}
	if cfg.SessionTimeout != 0 {
		out.WriteString(";sessionTimeout=")
		out.WriteString(cfg.SessionTimeout.String())
	}
	if !cfg.Auth.Enabled {
		return
	}
	if cfg.Auth.Scheme != "" {
		out.WriteString(";authtype=")
		out.WriteString(cfg.Auth.Scheme)
	}
	if cfg.Auth.Raw != nil {
		out.WriteString(";authdata=")
		out.WriteString(base64.StdEncoding.EncodeToString(cfg.Auth.Raw))
	}
	if cfg.Auth.Username != "" {
		out.WriteString(";username=")
		out.WriteString(cfg.Auth.Username)
	}
	if cfg.Auth.Password != "" {
		out.WriteString(";password=")
		out.WriteString(cfg.Auth.Password)
	}
}

// String returns the string representation.
func (cfg ZKConfig) String() string {
	if !cfg.Enabled {
		return ""
	}

	var buf strings.Builder
	buf.Grow(64)
	cfg.AppendTo(&buf)
	return buf.String()
}

// MarshalJSON fulfills json.Marshaler.
func (cfg ZKConfig) MarshalJSON() ([]byte, error) {
	if !cfg.Enabled {
		return constants.NullBytes, nil
	}
	return json.Marshal(cfg.ToJSON())
}

// MarshalJSON fulfills json.Marshaler.
func (cfg ZKAuthConfig) MarshalJSON() ([]byte, error) {
	if !cfg.Enabled {
		return constants.NullBytes, nil
	}
	return json.Marshal(cfg.ToJSON())
}

// ToJSON converts the object to its JSON doppelgänger.
func (cfg ZKConfig) ToJSON() *ZKConfigJSON {
	if !cfg.Enabled {
		return nil
	}
	return &ZKConfigJSON{
		Servers:        cfg.Servers,
		SessionTimeout: cfg.SessionTimeout,
		Auth:           cfg.Auth.ToJSON(),
	}
}

// ToJSON converts the object to its JSON doppelgänger.
func (cfg ZKAuthConfig) ToJSON() *ZKAuthConfigJSON {
	if !cfg.Enabled {
		return nil
	}
	return &ZKAuthConfigJSON{
		Scheme:   cfg.Scheme,
		Raw:      base64.StdEncoding.EncodeToString(cfg.Raw),
		Username: cfg.Username,
		Password: cfg.Password,
	}
}

// Parse parses the string representation.
func (cfg *ZKConfig) Parse(str string) error {
	if cfg == nil {
		panic(errors.New("*ZKConfig is nil"))
	}

	wantZero := true
	defer func() {
		if wantZero {
			*cfg = ZKConfig{}
		}
	}()

	if str == "" || str == constants.NullString {
		return nil
	}

	err := misc.StrictUnmarshalJSON([]byte(str), cfg)
	if err == nil {
		wantZero = false
		return nil
	}

	cfg.Enabled = true

	pieces := strings.Split(str, ";")

	if pieces[0] == "" {
		return nil
	}

	serverListString, err := roxyutil.ExpandString(pieces[0])
	if err != nil {
		return err
	}

	serverList := strings.Split(serverListString, ",")
	cfg.Servers = make([]string, 0, len(serverList))
	for _, server := range serverList {
		if server == "" {
			continue
		}
		cfg.Servers = append(cfg.Servers, server)
	}
	if len(cfg.Servers) == 0 {
		return nil
	}

	for _, item := range pieces[1:] {
		switch {
		case strings.HasPrefix(item, "sessionTimeout="):
			cfg.SessionTimeout, err = time.ParseDuration(item[15:])
			if err != nil {
				return err
			}

		case strings.HasPrefix(item, "authtype="):
			cfg.Auth.Enabled = true
			cfg.Auth.Scheme = item[9:]

		case strings.HasPrefix(item, "authdata="):
			cfg.Auth.Enabled = true
			cfg.Auth.Raw, err = misc.TryBase64DecodeString(item[9:])
			if err != nil {
				return err
			}

		case strings.HasPrefix(item, "username="):
			expanded, err := roxyutil.ExpandString(item[9:])
			if err != nil {
				return err
			}
			cfg.Auth.Enabled = true
			if cfg.Auth.Scheme == "" {
				cfg.Auth.Scheme = "digest"
			}
			cfg.Auth.Username = expanded

		case strings.HasPrefix(item, "password="):
			expanded, err := roxyutil.ExpandPassword(item[9:])
			if err != nil {
				return err
			}
			cfg.Auth.Enabled = true
			cfg.Auth.Password = expanded

		default:
			return fmt.Errorf("unknown option %q", item)
		}
	}

	err = cfg.PostProcess()
	if err != nil {
		return err
	}

	wantZero = false
	return nil
}

// UnmarshalJSON fulfills json.Unmarshaler.
func (cfg *ZKConfig) UnmarshalJSON(raw []byte) error {
	if cfg == nil {
		panic(errors.New("*ZKConfig is nil"))
	}

	wantZero := true
	defer func() {
		if wantZero {
			*cfg = ZKConfig{}
		}
	}()

	if bytes.Equal(raw, constants.NullBytes) {
		return nil
	}

	var alt *ZKConfigJSON
	err := misc.StrictUnmarshalJSON(raw, &alt)
	if err != nil {
		return err
	}

	err = cfg.FromJSON(alt)
	if err != nil {
		return err
	}

	err = cfg.PostProcess()
	if err != nil {
		return err
	}

	wantZero = false
	return nil
}

// UnmarshalJSON fulfills json.Unmarshaler.
func (cfg *ZKAuthConfig) UnmarshalJSON(raw []byte) error {
	if cfg == nil {
		panic(errors.New("*ZKAuthConfig is nil"))
	}

	wantZero := true
	defer func() {
		if wantZero {
			*cfg = ZKAuthConfig{}
		}
	}()

	if bytes.Equal(raw, constants.NullBytes) {
		return nil
	}

	var alt *ZKAuthConfigJSON
	err := misc.StrictUnmarshalJSON(raw, &alt)
	if err != nil {
		return err
	}

	err = cfg.FromJSON(alt)
	if err != nil {
		return err
	}

	err = cfg.PostProcess()
	if err != nil {
		return err
	}

	wantZero = false
	return nil
}

// FromJSON converts the object's JSON doppelgänger into the object.
func (cfg *ZKConfig) FromJSON(alt *ZKConfigJSON) error {
	if cfg == nil {
		panic(errors.New("*ZKConfig is nil"))
	}

	if alt == nil {
		*cfg = ZKConfig{}
		return nil
	}

	*cfg = ZKConfig{
		Enabled:        true,
		Servers:        alt.Servers,
		SessionTimeout: alt.SessionTimeout,
	}

	err := cfg.Auth.FromJSON(alt.Auth)
	if err != nil {
		return err
	}

	return nil
}

// FromJSON converts the object's JSON doppelgänger into the object.
func (cfg *ZKAuthConfig) FromJSON(alt *ZKAuthConfigJSON) error {
	if cfg == nil {
		panic(errors.New("*ZKAuthConfig is nil"))
	}

	if alt == nil {
		*cfg = ZKAuthConfig{}
		return nil
	}

	*cfg = ZKAuthConfig{
		Enabled:  true,
		Scheme:   alt.Scheme,
		Username: alt.Username,
		Password: alt.Password,
	}

	raw, err := misc.TryBase64DecodeString(alt.Raw)
	if err != nil {
		return err
	}

	cfg.Raw = raw
	return nil
}

// PostProcess performs data integrity checks and input post-processing.
func (cfg *ZKConfig) PostProcess() error {
	if cfg == nil {
		panic(errors.New("*ZKConfig is nil"))
	}

	if !cfg.Enabled {
		*cfg = ZKConfig{}
		return nil
	}

	if len(cfg.Servers) == 0 {
		return errors.New("len(ZKConfig.Servers) == 0")
	}

	for index := range cfg.Servers {
		server := cfg.Servers[index]
		if server == "" {
			return fmt.Errorf("ZKConfig.Servers[%d] == %q", index, server)
		}

		host, port, err := misc.SplitHostPort(server, constants.PortZK)
		if err != nil {
			return err
		}

		cfg.Servers[index] = net.JoinHostPort(host, port)
	}

	err := cfg.Auth.PostProcess()
	if err != nil {
		return err
	}

	return nil
}

// PostProcess performs data integrity checks and input post-processing.
func (cfg *ZKAuthConfig) PostProcess() error {
	if cfg == nil {
		panic(errors.New("*ZKAuthConfig is nil"))
	}

	if !cfg.Enabled {
		*cfg = ZKAuthConfig{}
		return nil
	}

	if cfg.Raw == nil && cfg.Username == "" {
		return errors.New("must specify either \"ZKAuthConfig.Raw\" or \"ZKAuthConfig.Username\"")
	}

	if cfg.Raw != nil && cfg.Username != "" {
		return errors.New("cannot specify both \"ZKAuthConfig.Raw\" and \"ZKAuthConfig.Username\"")
	}

	if cfg.Scheme == "" && cfg.Raw != nil {
		return errors.New("must specify \"ZKAuthConfig.Scheme\"")
	}

	if cfg.Scheme == "" {
		cfg.Scheme = "digest"
	}

	return nil
}

// Connect constructs the configured *zk.Conn and dials the ZooKeeper cluster.
func (cfg ZKConfig) Connect(ctx context.Context) (*zk.Conn, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	sessTimeout := cfg.SessionTimeout
	if sessTimeout == 0 {
		sessTimeout = 30 * time.Second
	}

	zkconn, _, err := zk.Connect(
		cfg.Servers,
		sessTimeout,
		zk.WithLogger(ZKLoggerBridge{}))
	if err != nil {
		return nil, err
	}

	if cfg.Auth.Enabled {
		var authRaw []byte
		if cfg.Auth.Raw != nil {
			authRaw = cfg.Auth.Raw
		} else {
			authRaw = []byte(cfg.Auth.Username + ":" + cfg.Auth.Password)
		}

		err = zkconn.AddAuth(cfg.Auth.Scheme, authRaw)
		if err != nil {
			zkconn.Close()
			return nil, fmt.Errorf("failed to (*zk.Conn).AddAuth with scheme=%q: %w", cfg.Auth.Scheme, err)
		}
	}

	return zkconn, nil
}
