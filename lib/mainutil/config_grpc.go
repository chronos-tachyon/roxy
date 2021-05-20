package mainutil

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/roxyresolver"
)

// GRPCClientConfig represents the configuration for a *grpc.ClientConn.
type GRPCClientConfig struct {
	Enabled bool
	Target  roxyresolver.Target
	TLS     TLSClientConfig
}

// GRPCClientConfigJSON represents the JSON doppelgänger of an GRPCClientConfig.
type GRPCClientConfigJSON struct {
	Target string               `json:"target"`
	TLS    *TLSClientConfigJSON `json:"tls,omitempty"`
}

// AppendTo appends the string representation to the given Builder.
func (cfg GRPCClientConfig) AppendTo(out *strings.Builder) {
	cfg.Target.AppendTo(out)
	if cfg.TLS.Enabled {
		out.WriteString(";tls=")
		cfg.TLS.AppendTo(out)
	}
}

// String returns the string representation.
func (cfg GRPCClientConfig) String() string {
	if !cfg.Enabled {
		return ""
	}

	var buf strings.Builder
	buf.Grow(64)
	cfg.AppendTo(&buf)
	return buf.String()
}

// MarshalJSON fulfills json.Marshaler.
func (cfg GRPCClientConfig) MarshalJSON() ([]byte, error) {
	if !cfg.Enabled {
		return constants.NullBytes, nil
	}
	return json.Marshal(cfg.ToJSON())
}

// ToJSON converts the object to its JSON doppelgänger.
func (cfg GRPCClientConfig) ToJSON() *GRPCClientConfigJSON {
	if !cfg.Enabled {
		return nil
	}
	return &GRPCClientConfigJSON{
		Target: cfg.Target.String(),
		TLS:    cfg.TLS.ToJSON(),
	}
}

// Parse parses the string representation.
func (cfg *GRPCClientConfig) Parse(str string) error {
	if cfg == nil {
		panic(errors.New("*GRPCClientConfig is nil"))
	}

	wantZero := true
	defer func() {
		if wantZero {
			*cfg = GRPCClientConfig{}
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

	err = cfg.Target.Parse(pieces[0])
	if err != nil {
		return err
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
func (cfg *GRPCClientConfig) UnmarshalJSON(raw []byte) error {
	if cfg == nil {
		panic(errors.New("*GRPCClientConfig is nil"))
	}

	wantZero := true
	defer func() {
		if wantZero {
			*cfg = GRPCClientConfig{}
		}
	}()

	if bytes.Equal(raw, constants.NullBytes) {
		return nil
	}

	var alt *GRPCClientConfigJSON
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
func (cfg *GRPCClientConfig) FromJSON(alt *GRPCClientConfigJSON) error {
	if cfg == nil {
		panic(errors.New("*GRPCClientConfig is nil"))
	}

	if alt == nil {
		*cfg = GRPCClientConfig{}
		return nil
	}

	*cfg = GRPCClientConfig{
		Enabled: true,
	}

	err := cfg.Target.Parse(alt.Target)
	if err != nil {
		return err
	}

	err = cfg.TLS.FromJSON(alt.TLS)
	if err != nil {
		return err
	}

	return nil
}

// PostProcess performs data integrity checks and input post-processing.
func (cfg *GRPCClientConfig) PostProcess() error {
	if cfg == nil {
		panic(errors.New("*GRPCClientConfig is nil"))
	}

	if !cfg.Enabled {
		*cfg = GRPCClientConfig{}
		return nil
	}

	err := cfg.TLS.PostProcess()
	if err != nil {
		return err
	}

	return nil
}

// Dial dials the configured gRPC server.
func (cfg GRPCClientConfig) Dial(ctx context.Context, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	dialOpts := make([]grpc.DialOption, 2, 2+len(opts))
	dialOpts[0] = roxyresolver.WithStandardResolvers(ctx)
	if cfg.TLS.Enabled {
		tlsConfig, err := cfg.TLS.MakeTLS(cfg.Target.ServerName)
		if err != nil {
			return nil, err
		}
		dialOpts[1] = grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))
	} else {
		dialOpts[1] = grpc.WithInsecure()
	}
	dialOpts = append(dialOpts, opts...)

	cc, err := grpc.DialContext(ctx, cfg.Target.String(), dialOpts...)
	if err != nil {
		return nil, err
	}

	return cc, nil
}
