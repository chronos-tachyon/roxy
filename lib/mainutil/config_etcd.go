package mainutil

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strings"
	"time"

	v3 "go.etcd.io/etcd/client/v3"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

// EtcdConfig represents the configuration for an etcd.io *v3.Client.
type EtcdConfig struct {
	Enabled          bool
	Endpoints        []string
	TLS              TLSClientConfig
	Username         string
	Password         string
	DialTimeout      time.Duration
	KeepAlive        time.Duration
	KeepAliveTimeout time.Duration
}

// EtcdConfigJSON represents the JSON doppelgänger of an EtcdConfig.
type EtcdConfigJSON struct {
	Endpoints        []string             `json:"endpoints"`
	TLS              *TLSClientConfigJSON `json:"tls,omitempty"`
	Username         string               `json:"username,omitempty"`
	Password         string               `json:"password,omitempty"`
	DialTimeout      time.Duration        `json:"dialTimeout,omitempty"`
	KeepAlive        time.Duration        `json:"keepAlive,omitempty"`
	KeepAliveTimeout time.Duration        `json:"keepAliveTimeout,omitempty"`
}

// AppendTo appends the string representation to the given Builder.
func (cfg EtcdConfig) AppendTo(out *strings.Builder) {
	if !cfg.Enabled {
		return
	}
	for i, endpoint := range cfg.Endpoints {
		if i != 0 {
			out.WriteString(",")
		}
		out.WriteString(endpoint)
	}
	if cfg.TLS.Enabled {
		out.WriteString(";tls=")
		cfg.TLS.AppendTo(out)
	}
	if cfg.Username != "" {
		out.WriteString(";username=")
		out.WriteString(cfg.Username)
	}
	if cfg.Password != "" {
		out.WriteString(";password=")
		out.WriteString(cfg.Password)
	}
	if cfg.DialTimeout != 0 {
		out.WriteString(";dialTimeout=")
		out.WriteString(cfg.DialTimeout.String())
	}
	if cfg.KeepAlive != 0 {
		out.WriteString(";keepAlive=")
		out.WriteString(cfg.KeepAlive.String())
	}
	if cfg.KeepAliveTimeout != 0 {
		out.WriteString(";keepAliveTimeout=")
		out.WriteString(cfg.KeepAliveTimeout.String())
	}
}

// String returns the string representation.
func (cfg EtcdConfig) String() string {
	if !cfg.Enabled {
		return ""
	}

	var buf strings.Builder
	buf.Grow(64)
	cfg.AppendTo(&buf)
	return buf.String()
}

// MarshalJSON fulfills json.Marshaler.
func (cfg EtcdConfig) MarshalJSON() ([]byte, error) {
	if !cfg.Enabled {
		return constants.NullBytes, nil
	}
	return json.Marshal(cfg.ToJSON())
}

// ToJSON converts the object to its JSON doppelgänger.
func (cfg EtcdConfig) ToJSON() *EtcdConfigJSON {
	if !cfg.Enabled {
		return nil
	}
	return &EtcdConfigJSON{
		Endpoints:        cfg.Endpoints,
		TLS:              cfg.TLS.ToJSON(),
		Username:         cfg.Username,
		Password:         cfg.Password,
		DialTimeout:      cfg.DialTimeout,
		KeepAlive:        cfg.KeepAlive,
		KeepAliveTimeout: cfg.KeepAliveTimeout,
	}
}

// Parse parses the string representation.
func (cfg *EtcdConfig) Parse(str string) error {
	if cfg == nil {
		panic(errors.New("*EtcdConfig is nil"))
	}

	wantZero := true
	defer func() {
		if wantZero {
			*cfg = EtcdConfig{}
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

	for _, item := range pieces[1:] {
		optName, optValue, optComplete, err := splitOption(item)
		if err != nil {
			return err
		}

		optErr := OptionError{
			Name:     optName,
			Value:    optValue,
			Complete: optComplete,
		}

		switch optName {
		case optionTLS:
			err = cfg.TLS.Parse(optValue)
			if err != nil {
				optErr.Err = err
				return optErr
			}

		case optionUsername:
			cfg.Username, err = roxyutil.ExpandString(optValue)
			if err != nil {
				optErr.Err = err
				return optErr
			}

		case optionPassword:
			cfg.Password, err = roxyutil.ExpandPassword(optValue)
			if err != nil {
				optErr.Err = err
				return optErr
			}

		case optionDialTimeout:
			cfg.DialTimeout, err = time.ParseDuration(optValue)
			if err != nil {
				optErr.Err = err
				return optErr
			}

		case optionKeepAlive:
			cfg.KeepAlive, err = time.ParseDuration(optValue)
			if err != nil {
				optErr.Err = err
				return optErr
			}

		case optionKeepAliveTimeout:
			cfg.KeepAliveTimeout, err = time.ParseDuration(optValue)
			if err != nil {
				optErr.Err = err
				return optErr
			}

		default:
			optErr.Err = UnknownOptionError{}
			return optErr
		}
	}

	endpointListString, err := roxyutil.ExpandString(pieces[0])
	if err != nil {
		return err
	}

	endpointList := strings.Split(endpointListString, ",")
	cfg.Endpoints = make([]string, 0, len(endpointList))
	for _, endpoint := range endpointList {
		if endpoint == "" {
			continue
		}
		cfg.Endpoints = append(cfg.Endpoints, endpoint)
	}

	err = cfg.PostProcess()
	if err != nil {
		return err
	}

	wantZero = false
	return nil
}

// UnmarshalJSON fulfills json.Unmarshaler.
func (cfg *EtcdConfig) UnmarshalJSON(raw []byte) error {
	if cfg == nil {
		panic(errors.New("*EtcdConfig is nil"))
	}

	wantZero := true
	defer func() {
		if wantZero {
			*cfg = EtcdConfig{}
		}
	}()

	if bytes.Equal(raw, constants.NullBytes) {
		return nil
	}

	var alt *EtcdConfigJSON
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
func (cfg *EtcdConfig) FromJSON(alt *EtcdConfigJSON) error {
	if cfg == nil {
		panic(errors.New("*EtcdConfig is nil"))
	}

	if alt == nil {
		*cfg = EtcdConfig{}
		return nil
	}

	*cfg = EtcdConfig{
		Enabled:          true,
		Endpoints:        alt.Endpoints,
		Username:         alt.Username,
		Password:         alt.Password,
		DialTimeout:      alt.DialTimeout,
		KeepAlive:        alt.KeepAlive,
		KeepAliveTimeout: alt.KeepAliveTimeout,
	}

	err := cfg.TLS.FromJSON(alt.TLS)
	if err != nil {
		return err
	}

	return nil
}

// PostProcess performs data integrity checks and input post-processing.
func (cfg *EtcdConfig) PostProcess() error {
	if cfg == nil {
		panic(errors.New("*EtcdConfig is nil"))
	}

	if !cfg.Enabled {
		*cfg = EtcdConfig{}
		return nil
	}

	if len(cfg.Endpoints) == 0 {
		return roxyutil.StructFieldError{
			Field: "EtcdConfig.Endpoints",
			Value: cfg.Endpoints,
			Err:   roxyutil.ErrExpectNonEmptyList,
		}
	}

	err := cfg.TLS.PostProcess()
	if err != nil {
		return err
	}

	expectScheme := constants.SchemeHTTP
	if cfg.TLS.Enabled {
		expectScheme = constants.SchemeHTTPS
	}

	hostnames := make([]string, len(cfg.Endpoints))
	for index := range cfg.Endpoints {
		endpoint := cfg.Endpoints[index]

		idxErr := roxyutil.ListIndexError{
			List:  "EtcdConfig.Endpoints",
			Index: uint(index),
			Value: endpoint,
		}

		if endpoint == "" {
			idxErr.Err = roxyutil.ErrExpectNonEmpty
			return idxErr
		}

		u, err := url.Parse(endpoint)
		if err != nil {
			idxErr.Err = err
			return idxErr
		}

		if u.Scheme == "" {
			u.Scheme = expectScheme
		}

		if u.Scheme != expectScheme {
			idxErr.Err = roxyutil.SchemeError{
				Scheme: u.Scheme,
				Err:    roxyutil.ExpectLiteralError(expectScheme),
			}
			return idxErr
		}

		if u.Port() == "" {
			u.Host = net.JoinHostPort(u.Hostname(), constants.PortEtcd)
		}

		if u.User != nil {
			idxErr.Err = roxyutil.StructFieldError{
				Field: "URL.User",
				Value: u.User.String(),
				Err:   roxyutil.ErrExpectEmpty,
			}
			return idxErr
		}
		if u.Path != "" {
			idxErr.Err = roxyutil.StructFieldError{
				Field: "URL.Path",
				Value: u.Path,
				Err:   roxyutil.ErrExpectEmpty,
			}
			return idxErr
		}
		if u.RawQuery != "" {
			idxErr.Err = roxyutil.StructFieldError{
				Field: "URL.RawQuery",
				Value: u.RawQuery,
				Err:   roxyutil.ErrExpectEmpty,
			}
			return idxErr
		}
		if u.RawFragment != "" {
			idxErr.Err = roxyutil.StructFieldError{
				Field: "URL.RawFragment",
				Value: u.RawFragment,
				Err:   roxyutil.ErrExpectEmpty,
			}
			return idxErr
		}

		cfg.Endpoints[index] = u.String()
		hostnames[index] = u.Hostname()
	}

	if cfg.TLS.Enabled && !cfg.TLS.SkipVerify && !cfg.TLS.SkipVerifyServerName && cfg.TLS.ServerName == "" {
		cfg.TLS.ServerName = hostnames[0]
	}

	return nil
}

// Connect constructs the configured etcd.io *v3.Client and dials the etcd
// cluster.
func (cfg EtcdConfig) Connect(ctx context.Context) (*v3.Client, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	tlsConfig, err := cfg.TLS.MakeTLS("")
	if err != nil {
		return nil, err
	}

	dialTimeout := cfg.DialTimeout
	if dialTimeout == 0 {
		dialTimeout = 5 * time.Second
	}

	return v3.New(v3.Config{
		Endpoints:            cfg.Endpoints,
		AutoSyncInterval:     1 * time.Minute,
		DialTimeout:          dialTimeout,
		DialKeepAliveTime:    cfg.KeepAlive,
		DialKeepAliveTimeout: cfg.KeepAliveTimeout,
		Username:             cfg.Username,
		Password:             cfg.Password,
		TLS:                  tlsConfig,
		LogConfig:            NewDummyZapConfig(),
		Context:              ctx,
	})
}
