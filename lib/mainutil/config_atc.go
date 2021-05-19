package mainutil

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/atcclient"
)

// ATCClientConfig represents the configuration for an *atcclient.ATCClient.
type ATCClientConfig struct {
	GRPCClientConfig
}

// ATCClientConfigJSON represents the JSON doppelgänger of an ATCClientConfig.
type ATCClientConfigJSON struct {
	GRPCClientConfigJSON
}

// AppendTo appends the string representation to the given Builder.
func (cfg ATCClientConfig) AppendTo(out *strings.Builder) {
	cfg.GRPCClientConfig.AppendTo(out)
}

// String returns the string representation.
func (cfg ATCClientConfig) String() string {
	return cfg.GRPCClientConfig.String()
}

// MarshalJSON fulfills json.Marshaler.
func (cfg ATCClientConfig) MarshalJSON() ([]byte, error) {
	if !cfg.Enabled {
		return constants.NullBytes, nil
	}
	return json.Marshal(cfg.ToJSON())
}

// ToJSON converts the object to its JSON doppelgänger.
func (cfg ATCClientConfig) ToJSON() *ATCClientConfigJSON {
	if !cfg.Enabled {
		return nil
	}
	return &ATCClientConfigJSON{
		GRPCClientConfigJSON: *cfg.GRPCClientConfig.ToJSON(),
	}
}

// Parse parses the string representation.
func (cfg *ATCClientConfig) Parse(str string) error {
	if cfg == nil {
		panic(errors.New("*ATCClientConfig is nil"))
	}

	return cfg.GRPCClientConfig.Parse(str)
}

// UnmarshalJSON fulfills json.Unmarshaler.
func (cfg *ATCClientConfig) UnmarshalJSON(raw []byte) error {
	if cfg == nil {
		panic(errors.New("*ATCClientConfig is nil"))
	}

	wantZero := true
	defer func() {
		if wantZero {
			*cfg = ATCClientConfig{}
		}
	}()

	if bytes.Equal(raw, constants.NullBytes) {
		return nil
	}

	var alt *ATCClientConfigJSON
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
func (cfg *ATCClientConfig) FromJSON(alt *ATCClientConfigJSON) error {
	if cfg == nil {
		panic(errors.New("*ATCClientConfig is nil"))
	}

	if alt == nil {
		*cfg = ATCClientConfig{}
		return nil
	}

	err := cfg.GRPCClientConfig.FromJSON(&alt.GRPCClientConfigJSON)
	if err != nil {
		return err
	}

	return nil
}

// PostProcess performs data integrity checks and input post-processing.
func (cfg *ATCClientConfig) PostProcess() error {
	if cfg == nil {
		panic(errors.New("*ATCClientConfig is nil"))
	}

	return cfg.GRPCClientConfig.PostProcess()
}

// NewClient constructs the configured *atcclient.ATCClient and dials the ATC
// server.
func (cfg ATCClientConfig) NewClient(ctx context.Context) (*atcclient.ATCClient, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	tlsConfig, err := cfg.TLS.MakeTLS("")
	if err != nil {
		return nil, err
	}

	cc, err := cfg.Dial(ctx)
	if err != nil {
		return nil, err
	}

	return atcclient.New(cc, tlsConfig)
}
