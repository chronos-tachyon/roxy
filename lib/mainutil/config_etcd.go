package mainutil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	v3 "go.etcd.io/etcd/client/v3"
)

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

type ecJSON struct {
	Endpoints        []string      `json:"endpoints"`
	TLS              *tccJSON      `json:"tls,omitempty"`
	Username         string        `json:"username,omitempty"`
	Password         string        `json:"password,omitempty"`
	DialTimeout      time.Duration `json:"dialTimeout,omitempty"`
	KeepAlive        time.Duration `json:"keepAlive,omitempty"`
	KeepAliveTimeout time.Duration `json:"keepAliveTimeout,omitempty"`
}

func (ec EtcdConfig) AppendTo(out *strings.Builder) {
	if !ec.Enabled {
		return
	}
	for i, endpoint := range ec.Endpoints {
		if i != 0 {
			out.WriteString(",")
		}
		out.WriteString(endpoint)
	}
	if ec.TLS.Enabled {
		out.WriteString(";tls=")
		ec.TLS.AppendTo(out)
	}
	if ec.Username != "" {
		out.WriteString(";username=")
		out.WriteString(ec.Username)
	}
	if ec.Password != "" {
		out.WriteString(";password=")
		out.WriteString(ec.Password)
	}
	if ec.DialTimeout != 0 {
		out.WriteString(";dialTimeout=")
		out.WriteString(ec.DialTimeout.String())
	}
	if ec.KeepAlive != 0 {
		out.WriteString(";keepAlive=")
		out.WriteString(ec.KeepAlive.String())
	}
	if ec.KeepAliveTimeout != 0 {
		out.WriteString(";keepAliveTimeout=")
		out.WriteString(ec.KeepAliveTimeout.String())
	}
}

func (ec EtcdConfig) String() string {
	if !ec.Enabled {
		return ""
	}

	var buf strings.Builder
	buf.Grow(64)
	ec.AppendTo(&buf)
	return buf.String()
}

func (ec EtcdConfig) MarshalJSON() ([]byte, error) {
	if !ec.Enabled {
		return nullBytes, nil
	}
	return json.Marshal(ec.toAlt())
}

func (ec *EtcdConfig) Parse(str string) error {
	wantZero := true
	defer func() {
		if wantZero {
			*ec = EtcdConfig{}
		}
	}()

	if str == "" || str == nullString {
		return nil
	}

	err := json.Unmarshal([]byte(str), ec)
	if err == nil {
		wantZero = false
		return nil
	}

	pieces := strings.Split(str, ";")

	if pieces[0] == "" {
		return nil
	}

	for _, item := range pieces[1:] {
		switch {
		case strings.HasPrefix(item, "tls="):
			err = ec.TLS.Parse(item[4:])
			if err != nil {
				return err
			}

		case strings.HasPrefix(item, "username="):
			ec.Username = item[9:]

		case strings.HasPrefix(item, "password="):
			ec.Password = item[9:]

		case strings.HasPrefix(item, "dialTimeout="):
			ec.DialTimeout, err = time.ParseDuration(item[12:])
			if err != nil {
				return err
			}

		case strings.HasPrefix(item, "keepAlive="):
			ec.KeepAlive, err = time.ParseDuration(item[10:])
			if err != nil {
				return err
			}

		case strings.HasPrefix(item, "keepAliveTimeout="):
			ec.KeepAliveTimeout, err = time.ParseDuration(item[17:])
			if err != nil {
				return err
			}

		default:
			return fmt.Errorf("unknown option %q", item)
		}
	}

	expectScheme := "http"
	if ec.TLS.Enabled {
		expectScheme = "https"
	}

	endpointList := strings.Split(pieces[0], ",")
	ec.Endpoints = make([]string, 0, len(endpointList))
	for _, endpoint := range endpointList {
		if endpoint == "" {
			continue
		}
		u, err := url.Parse(endpoint)
		if err != nil {
			return fmt.Errorf("failed to parse endpoint URL %q: %w", endpoint, err)
		}
		if u.Scheme == "" {
			u.Scheme = expectScheme
		}
		if u.Scheme != expectScheme {
			return fmt.Errorf("expected scheme %q, got scheme %q in URL %q", expectScheme, u.Scheme, u.String())
		}
		if u.Port() == "" {
			u.Host = net.JoinHostPort(u.Hostname(), "2379")
		}
		if u.User != nil || u.Path != "" || u.RawQuery != "" || u.RawFragment != "" {
			return fmt.Errorf("URL %q contains forbidden components", u.String())
		}
		ec.Endpoints = append(ec.Endpoints, u.String())
	}

	ec.Enabled = true
	tmp, err := ec.postprocess()
	if err != nil {
		return err
	}

	*ec = tmp
	wantZero = false
	return nil
}

func (ec *EtcdConfig) UnmarshalJSON(raw []byte) error {
	wantZero := true
	defer func() {
		if wantZero {
			*ec = EtcdConfig{}
		}
	}()

	if bytes.Equal(raw, nullBytes) {
		return nil
	}

	var alt ecJSON
	err := json.Unmarshal(raw, &alt)
	if err != nil {
		return err
	}

	tmp, err := alt.toStd().postprocess()
	if err != nil {
		return err
	}

	*ec = tmp
	wantZero = false
	return nil
}

func (ec EtcdConfig) Connect(ctx context.Context) (*v3.Client, error) {
	if !ec.Enabled {
		return nil, nil
	}

	serverName, err := serverNameFromEtcdConfig(ec)
	if err != nil {
		return nil, err
	}

	tlsConfig, err := ec.TLS.MakeTLS(serverName)
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate TLS config: %w", err)
	}

	dialTimeout := ec.DialTimeout
	if dialTimeout == 0 {
		dialTimeout = 5 * time.Second
	}

	return v3.New(v3.Config{
		Endpoints:            ec.Endpoints,
		AutoSyncInterval:     1 * time.Minute,
		DialTimeout:          dialTimeout,
		DialKeepAliveTime:    ec.KeepAlive,
		DialKeepAliveTimeout: ec.KeepAliveTimeout,
		Username:             ec.Username,
		Password:             ec.Password,
		TLS:                  tlsConfig,
		LogConfig:            NewDummyZapConfig(),
		Context:              ctx,
	})
}

func (ec EtcdConfig) toAlt() *ecJSON {
	if !ec.Enabled {
		return nil
	}
	return &ecJSON{
		Endpoints:        ec.Endpoints,
		TLS:              ec.TLS.toAlt(),
		Username:         ec.Username,
		Password:         ec.Password,
		DialTimeout:      ec.DialTimeout,
		KeepAlive:        ec.KeepAlive,
		KeepAliveTimeout: ec.KeepAliveTimeout,
	}
}

func (alt *ecJSON) toStd() EtcdConfig {
	if alt == nil {
		return EtcdConfig{}
	}
	return EtcdConfig{
		Enabled:          true,
		Endpoints:        alt.Endpoints,
		TLS:              alt.TLS.toStd(),
		Username:         alt.Username,
		Password:         alt.Password,
		DialTimeout:      alt.DialTimeout,
		KeepAlive:        alt.KeepAlive,
		KeepAliveTimeout: alt.KeepAliveTimeout,
	}
}

func (ec EtcdConfig) postprocess() (out EtcdConfig, err error) {
	defer func() {
		log.Logger.Trace().
			Interface("result", out).
			Msg("EtcdConfig parse result")
	}()

	var zero EtcdConfig

	if !ec.Enabled {
		return zero, nil
	}

	if len(ec.Endpoints) == 0 {
		return zero, nil
	}

	for _, endpoint := range ec.Endpoints {
		if endpoint == "" {
			return zero, nil
		}
	}

	if ec.TLS.Enabled && ec.TLS.ServerName == "" {
		u, err := url.Parse(ec.Endpoints[0])
		if err != nil {
			return zero, err
		}
		ec.TLS.ServerName = u.Hostname()
	}

	return ec, nil
}

func serverNameFromEtcdConfig(ec EtcdConfig) (string, error) {
	if !ec.Enabled || !ec.TLS.Enabled {
		return "", nil
	}
	u, err := url.Parse(ec.Endpoints[0])
	if err != nil {
		return "", err
	}
	serverName := u.Hostname()
	return serverName, nil
}
