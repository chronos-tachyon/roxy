package mainutil

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net"
	"strings"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

// ListenConfig represents the configuration for a net.Listener socket with
// optional TLS.
type ListenConfig struct {
	Enabled bool
	Network string
	Address string
	TLS     TLSServerConfig
}

// ListenConfigJSON represents the JSON doppelgänger of an ListenConfig.
type ListenConfigJSON struct {
	Network string               `json:"network"`
	Address string               `json:"address"`
	TLS     *TLSServerConfigJSON `json:"tls,omitempty"`
}

// AppendTo appends the string representation to the given Builder.
func (cfg ListenConfig) AppendTo(out *strings.Builder) {
	out.WriteString(cfg.Address)
	if cfg.Network != constants.NetTCP {
		out.WriteString(";net=")
		out.WriteString(cfg.Network)
	}
	if cfg.TLS.Enabled {
		out.WriteString(";tls=")
		cfg.TLS.AppendTo(out)
	}
}

// String returns the string representation.
func (cfg ListenConfig) String() string {
	if !cfg.Enabled {
		return ""
	}

	var buf strings.Builder
	buf.Grow(64)
	cfg.AppendTo(&buf)
	return buf.String()
}

// MarshalJSON fulfills json.Marshaler.
func (cfg ListenConfig) MarshalJSON() ([]byte, error) {
	if !cfg.Enabled {
		return constants.NullBytes, nil
	}
	return json.Marshal(cfg.ToJSON())
}

// ToJSON converts the object to its JSON doppelgänger.
func (cfg ListenConfig) ToJSON() *ListenConfigJSON {
	if !cfg.Enabled {
		return nil
	}
	return &ListenConfigJSON{
		Network: cfg.Network,
		Address: escapeListenAddress(cfg.Address),
		TLS:     cfg.TLS.ToJSON(),
	}
}

// Parse parses the string representation.
func (cfg *ListenConfig) Parse(str string) error {
	if cfg == nil {
		panic(errors.New("*ListenConfig is nil"))
	}

	wantZero := true
	defer func() {
		if wantZero {
			*cfg = ListenConfig{}
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

	cfg.Address = pieces[0]

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
		case optionNet:
			fallthrough
		case optionNetwork:
			cfg.Network = optValue

		case optionTLS:
			err = cfg.TLS.Parse(optValue)
			if err != nil {
				optErr.Err = err
				return optErr
			}

		default:
			optErr.Err = UnknownOptionError{}
			return optErr
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
func (cfg *ListenConfig) UnmarshalJSON(raw []byte) error {
	if cfg == nil {
		panic(errors.New("*ListenConfig is nil"))
	}

	wantZero := true
	defer func() {
		if wantZero {
			*cfg = ListenConfig{}
		}
	}()

	if bytes.Equal(raw, constants.NullBytes) {
		return nil
	}

	var alt *ListenConfigJSON
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
func (cfg *ListenConfig) FromJSON(alt *ListenConfigJSON) error {
	if cfg == nil {
		panic(errors.New("*ListenConfig is nil"))
	}

	if alt == nil {
		*cfg = ListenConfig{}
		return nil
	}

	*cfg = ListenConfig{
		Enabled: true,
		Network: alt.Network,
		Address: unescapeListenAddress(alt.Address),
	}

	err := cfg.TLS.FromJSON(alt.TLS)
	if err != nil {
		return err
	}

	return nil
}

// PostProcess performs data integrity checks and input post-processing.
func (cfg *ListenConfig) PostProcess() error {
	if cfg == nil {
		panic(errors.New("*ListenConfig is nil"))
	}

	if !cfg.Enabled {
		*cfg = ListenConfig{}
		return nil
	}

	err := cfg.TLS.PostProcess()
	if err != nil {
		return err
	}

	if cfg.Address == "" {
		return roxyutil.StructFieldError{
			Field: "ListenConfig.Address",
			Value: cfg.Address,
			Err:   roxyutil.ErrExpectNonEmpty,
		}
	}

	maybeUnix := (cfg.Network == constants.NetEmpty) || constants.IsNetUnix(cfg.Network)
	if maybeUnix {
		if cfg.Address[0] == '/' || cfg.Address[0] == '\x00' {
			if cfg.Network == constants.NetEmpty {
				cfg.Network = constants.NetUnix
			}
		} else if cfg.Address[0] == '@' {
			cfg.Address = "\x00" + cfg.Address[1:]
			if cfg.Network == constants.NetEmpty {
				cfg.Network = constants.NetUnix
			}
		} else if cfg.Network != constants.NetEmpty || strings.Contains(cfg.Address, "/") {
			abs, err := roxyutil.PathAbs(cfg.Address)
			if err != nil {
				return err
			}
			cfg.Address = abs
			if cfg.Network == constants.NetEmpty {
				cfg.Network = constants.NetUnix
			}
		}
	}

	if cfg.Network == constants.NetEmpty {
		cfg.Network = constants.NetTCP
	}

	return nil
}

// Listen creates the configured net.Listener.
func (cfg ListenConfig) Listen(ctx context.Context) (net.Listener, error) {
	if !cfg.Enabled {
		return newDummyListener(), nil
	}

	tlsConfig, err := cfg.TLS.MakeTLS()
	if err != nil {
		return nil, err
	}

	var l net.Listener
	l, err = net.Listen(cfg.Network, cfg.Address)
	if err != nil {
		return nil, err
	}

	if tlsConfig != nil {
		l = tls.NewListener(l, tlsConfig)
	}

	return l, nil
}

// ListenNoTLS creates the configured net.Listener.  TLS handshaking is not
// added automatically.
func (cfg ListenConfig) ListenNoTLS(ctx context.Context) (net.Listener, error) {
	if !cfg.Enabled {
		return newDummyListener(), nil
	}

	l, err := net.Listen(cfg.Network, cfg.Address)
	if err != nil {
		return nil, err
	}

	return l, nil
}

func escapeListenAddress(addr string) string {
	if addr != "" && addr[0] == '\x00' {
		return "@" + addr[1:]
	}
	return addr
}

func unescapeListenAddress(addr string) string {
	if addr != "" && addr[0] == '@' {
		return "\x00" + addr[1:]
	}
	return addr
}

// type dummyListener {{{

type dummyListener struct {
	ch chan struct{}
}

func newDummyListener() net.Listener {
	return dummyListener{ch: make(chan struct{})}
}

func (l dummyListener) Addr() net.Addr {
	return &net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 0,
	}
}

func (l dummyListener) Accept() (net.Conn, error) {
	<-l.ch
	return nil, net.ErrClosed
}

func (l dummyListener) Close() error {
	close(l.ch)
	return nil
}

var _ net.Listener = dummyListener{}

// }}}
