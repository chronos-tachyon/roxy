package mainutil

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

type ListenConfig struct {
	Enabled bool
	Network string
	Address string
	TLS     TLSServerConfig
}

type lcJSON struct {
	Network string   `json:"network"`
	Address string   `json:"address"`
	TLS     *tscJSON `json:"tls,omitempty"`
}

func (lc ListenConfig) AppendTo(out *strings.Builder) {
	out.WriteString(lc.Address)
	if lc.Network != constants.NetTCP {
		out.WriteString(";net=")
		out.WriteString(lc.Network)
	}
	if lc.TLS.Enabled {
		out.WriteString(";tls=")
		lc.TLS.AppendTo(out)
	}
}

func (lc ListenConfig) String() string {
	if !lc.Enabled {
		return ""
	}

	var buf strings.Builder
	buf.Grow(64)
	lc.AppendTo(&buf)
	return buf.String()
}

func (lc ListenConfig) MarshalJSON() ([]byte, error) {
	if !lc.Enabled {
		return constants.NullBytes, nil
	}
	return json.Marshal(lc.toAlt())
}

func (lc *ListenConfig) Parse(str string) error {
	wantZero := true
	defer func() {
		if wantZero {
			*lc = ListenConfig{}
		}
	}()

	if str == "" || str == constants.NullString {
		return nil
	}

	err := misc.StrictUnmarshalJSON([]byte(str), lc)
	if err == nil {
		wantZero = false
		return nil
	}

	pieces := strings.Split(str, ";")

	lc.Address = pieces[0]

	for _, item := range pieces[1:] {
		switch {
		case strings.HasPrefix(item, "net="):
			lc.Network = item[4:]

		case strings.HasPrefix(item, "tls="):
			err = lc.TLS.Parse(item[4:])
			if err != nil {
				return err
			}

		default:
			return fmt.Errorf("unknown option %q", item)
		}
	}

	lc.Enabled = true
	tmp, err := lc.postprocess()
	if err != nil {
		return err
	}

	*lc = tmp
	wantZero = false
	return nil
}

func (lc *ListenConfig) UnmarshalJSON(raw []byte) error {
	wantZero := true
	defer func() {
		if wantZero {
			*lc = ListenConfig{}
		}
	}()

	if bytes.Equal(raw, constants.NullBytes) {
		return nil
	}

	var alt lcJSON
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

	*lc = tmp2
	wantZero = false
	return nil
}

func (lc ListenConfig) Listen(ctx context.Context) (net.Listener, error) {
	if !lc.Enabled {
		return newDummyListener(), nil
	}

	tlsConfig, err := lc.TLS.MakeTLS()
	if err != nil {
		return nil, err
	}

	var l net.Listener
	l, err = net.Listen(lc.Network, lc.Address)
	if err != nil {
		return nil, err
	}

	if tlsConfig != nil {
		l = tls.NewListener(l, tlsConfig)
	}

	return l, nil
}

func (lc ListenConfig) ListenNoTLS(ctx context.Context) (net.Listener, error) {
	if !lc.Enabled {
		return newDummyListener(), nil
	}

	l, err := net.Listen(lc.Network, lc.Address)
	if err != nil {
		return nil, err
	}

	return l, nil
}

func (lc ListenConfig) toAlt() *lcJSON {
	if !lc.Enabled {
		return nil
	}
	return &lcJSON{
		Network: lc.Network,
		Address: escapeListenAddress(lc.Address),
		TLS:     lc.TLS.toAlt(),
	}
}

func (alt *lcJSON) toStd() (ListenConfig, error) {
	if alt == nil {
		return ListenConfig{}, nil
	}

	return ListenConfig{
		Enabled: true,
		Network: alt.Network,
		Address: unescapeListenAddress(alt.Address),
		TLS:     alt.TLS.toStd(),
	}, nil
}

func (lc ListenConfig) postprocess() (out ListenConfig, err error) {
	var zero ListenConfig

	if !lc.Enabled {
		return zero, nil
	}

	if lc.Address == "" {
		return zero, fmt.Errorf("invalid address %q: %w", lc.Address, roxyutil.ErrExpectNonEmpty)
	}

	maybeUnix := (lc.Network == constants.NetEmpty) || constants.IsNetUnix(lc.Network)
	if maybeUnix {
		if lc.Address[0] == '/' || lc.Address[0] == '\x00' {
			if lc.Network == constants.NetEmpty {
				lc.Network = constants.NetUnix
			}
		} else if lc.Address[0] == '@' {
			lc.Address = "\x00" + lc.Address[1:]
			if lc.Network == constants.NetEmpty {
				lc.Network = constants.NetUnix
			}
		} else if lc.Network != constants.NetEmpty || strings.Contains(lc.Address, "/") {
			abs, err := roxyutil.PathAbs(lc.Address)
			if err != nil {
				return zero, err
			}
			lc.Address = abs
			if lc.Network == constants.NetEmpty {
				lc.Network = constants.NetUnix
			}
		}
	}

	if lc.Network == constants.NetEmpty {
		lc.Network = constants.NetTCP
	}

	return lc, nil
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
