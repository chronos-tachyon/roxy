package mainutil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/roxyresolver"
)

type RoxyTarget = roxyresolver.RoxyTarget

type GRPCClientConfig struct {
	Enabled bool
	Target  RoxyTarget
	TLS     TLSClientConfig
}

type gccJSON struct {
	Target string   `json:"target"`
	TLS    *tccJSON `json:"tls,omitempty"`
}

func (gcc GRPCClientConfig) AppendTo(out *strings.Builder) {
	gcc.Target.AppendTo(out)
	if gcc.TLS.Enabled {
		out.WriteString(";tls=")
		gcc.TLS.AppendTo(out)
	}
}

func (gcc GRPCClientConfig) String() string {
	if !gcc.Enabled {
		return ""
	}

	var buf strings.Builder
	buf.Grow(64)
	gcc.AppendTo(&buf)
	return buf.String()
}

func (gcc GRPCClientConfig) MarshalJSON() ([]byte, error) {
	if !gcc.Enabled {
		return nullBytes, nil
	}
	return json.Marshal(gcc.toAlt())
}

func (gcc *GRPCClientConfig) Parse(str string) error {
	wantZero := true
	defer func() {
		if wantZero {
			*gcc = GRPCClientConfig{}
		}
	}()

	if str == "" || str == nullString {
		return nil
	}

	err := misc.StrictUnmarshalJSON([]byte(str), gcc)
	if err == nil {
		wantZero = false
		return nil
	}

	pieces := strings.Split(str, ";")

	err = gcc.Target.Parse(pieces[0])
	if err != nil {
		return err
	}

	for _, item := range pieces[1:] {
		switch {
		case strings.HasPrefix(item, "tls="):
			err = gcc.TLS.Parse(item[4:])
			if err != nil {
				return err
			}

		default:
			return fmt.Errorf("unknown option %q", item)
		}
	}

	gcc.Enabled = true
	tmp, err := gcc.postprocess()
	if err != nil {
		return err
	}

	*gcc = tmp
	wantZero = false
	return nil
}

func (gcc *GRPCClientConfig) UnmarshalJSON(raw []byte) error {
	wantZero := true
	defer func() {
		if wantZero {
			*gcc = GRPCClientConfig{}
		}
	}()

	if bytes.Equal(raw, nullBytes) {
		return nil
	}

	var alt gccJSON
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

	*gcc = tmp2
	wantZero = false
	return nil
}

func (gcc GRPCClientConfig) Dial(ctx context.Context, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	if !gcc.Enabled {
		return nil, nil
	}

	dialOpts := make([]grpc.DialOption, 2, 2+len(opts))
	dialOpts[0] = roxyresolver.WithStandardResolvers(ctx)
	if gcc.TLS.Enabled {
		tlsConfig, err := gcc.TLS.MakeTLS(gcc.Target.ServerName)
		if err != nil {
			return nil, err
		}
		dialOpts[1] = grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))
	} else {
		dialOpts[1] = grpc.WithInsecure()
	}
	dialOpts = append(dialOpts, opts...)

	cc, err := grpc.DialContext(ctx, gcc.Target.String(), dialOpts...)
	if err != nil {
		return nil, err
	}

	return cc, nil
}

func (gcc GRPCClientConfig) toAlt() *gccJSON {
	if !gcc.Enabled {
		return nil
	}
	return &gccJSON{
		Target: gcc.Target.String(),
		TLS:    gcc.TLS.toAlt(),
	}
}

func (alt *gccJSON) toStd() (GRPCClientConfig, error) {
	if alt == nil {
		return GRPCClientConfig{}, nil
	}

	var rt RoxyTarget
	err := rt.Parse(alt.Target)
	if err != nil {
		return GRPCClientConfig{}, err
	}

	return GRPCClientConfig{
		Enabled: true,
		Target:  rt,
		TLS:     alt.TLS.toStd(),
	}, nil
}

func (gcc GRPCClientConfig) postprocess() (out GRPCClientConfig, err error) {
	var zero GRPCClientConfig

	if !gcc.Enabled {
		return zero, nil
	}

	tmp, err := gcc.TLS.postprocess()
	if err != nil {
		return zero, err
	}
	gcc.TLS = tmp

	return gcc, nil
}
