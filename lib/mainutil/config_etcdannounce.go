package mainutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	v3 "go.etcd.io/etcd/client/v3"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/announcer"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

// EtcdAnnounceConfig represents the configuration for an etcd.io *v3.Client,
// plus the fields needed to call announcer.NewEtcd.
type EtcdAnnounceConfig struct {
	EtcdConfig
	Path      string
	Hostname  string
	NamedPort string
	Format    announcer.Format
}

// EtcdAnnounceConfigJSON represents the JSON doppelgänger of an EtcdAnnounceConfig.
type EtcdAnnounceConfigJSON struct {
	EtcdConfigJSON
	Path      string           `json:"path"`
	Hostname  string           `json:"hostname"`
	NamedPort string           `json:"namedPort"`
	Format    announcer.Format `json:"format"`
}

// AppendTo appends the string representation to the given Builder.
func (cfg EtcdAnnounceConfig) AppendTo(out *strings.Builder) {
	if !cfg.Enabled {
		return
	}
	cfg.EtcdConfig.AppendTo(out)
	out.WriteString(";path=")
	out.WriteString(cfg.Path)
	if cfg.Hostname != "" {
		out.WriteString(";hostname=")
		out.WriteString(cfg.Hostname)
	}
	if cfg.NamedPort != "" {
		out.WriteString(";port=")
		out.WriteString(cfg.NamedPort)
	}
	out.WriteString(";format=")
	out.WriteString(cfg.Format.String())
}

// String returns the string representation.
func (cfg EtcdAnnounceConfig) String() string {
	if !cfg.Enabled {
		return ""
	}

	var buf strings.Builder
	buf.Grow(64)
	cfg.AppendTo(&buf)
	return buf.String()
}

// MarshalJSON fulfills json.Marshaler.
func (cfg EtcdAnnounceConfig) MarshalJSON() ([]byte, error) {
	if !cfg.Enabled {
		return constants.NullBytes, nil
	}
	return json.Marshal(cfg.ToJSON())
}

// ToJSON converts the object to its JSON doppelgänger.
func (cfg EtcdAnnounceConfig) ToJSON() *EtcdAnnounceConfigJSON {
	if !cfg.Enabled {
		return nil
	}
	return &EtcdAnnounceConfigJSON{
		EtcdConfigJSON: *cfg.EtcdConfig.ToJSON(),
		Path:           cfg.Path,
		NamedPort:      cfg.NamedPort,
		Format:         cfg.Format,
	}
}

// Parse parses the string representation.
func (cfg *EtcdAnnounceConfig) Parse(str string) error {
	if cfg == nil {
		panic(errors.New("*EtcdAnnounceConfig is nil"))
	}

	wantZero := true
	defer func() {
		if wantZero {
			*cfg = EtcdAnnounceConfig{}
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

	pieces := strings.Split(str, ";")

	var rest strings.Builder
	rest.Grow(len(str))
	rest.WriteString(pieces[0])

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
		case optionPath:
			cfg.Path, err = roxyutil.ExpandString(optValue)
			if err != nil {
				optErr.Err = err
				return optErr
			}

		case optionHostID:
			cfg.Hostname, err = roxyutil.ExpandString(optValue)
			if err != nil {
				optErr.Err = err
				return optErr
			}

		case optionPort:
			cfg.NamedPort, err = roxyutil.ExpandString(optValue)
			if err != nil {
				optErr.Err = err
				return optErr
			}

		case optionFormat:
			err = cfg.Format.Parse(optValue)
			if err != nil {
				optErr.Err = err
				return optErr
			}

		default:
			rest.WriteString(";")
			rest.WriteString(item)
		}
	}

	err = cfg.EtcdConfig.Parse(rest.String())
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
func (cfg *EtcdAnnounceConfig) UnmarshalJSON(raw []byte) error {
	if cfg == nil {
		panic(errors.New("*EtcdAnnounceConfig is nil"))
	}

	wantZero := true
	defer func() {
		if wantZero {
			*cfg = EtcdAnnounceConfig{}
		}
	}()

	if bytes.Equal(raw, constants.NullBytes) {
		return nil
	}

	var alt *EtcdAnnounceConfigJSON
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
func (cfg *EtcdAnnounceConfig) FromJSON(alt *EtcdAnnounceConfigJSON) error {
	if cfg == nil {
		panic(errors.New("*EtcdAnnounceConfig is nil"))
	}

	if alt == nil {
		*cfg = EtcdAnnounceConfig{}
		return nil
	}

	*cfg = EtcdAnnounceConfig{
		Path:      alt.Path,
		NamedPort: alt.NamedPort,
		Format:    alt.Format,
	}

	err := cfg.EtcdConfig.FromJSON(&alt.EtcdConfigJSON)
	if err != nil {
		return err
	}

	return nil
}

// PostProcess performs data integrity checks and input post-processing.
func (cfg *EtcdAnnounceConfig) PostProcess() error {
	if cfg == nil {
		panic(errors.New("*EtcdAnnounceConfig is nil"))
	}

	if !cfg.Enabled {
		*cfg = EtcdAnnounceConfig{}
		return nil
	}

	err := cfg.EtcdConfig.PostProcess()
	if err != nil {
		return err
	}

	if !strings.HasSuffix(cfg.Path, "/") {
		cfg.Path += "/"
	}
	err = roxyutil.ValidateEtcdPath(cfg.Path)
	if err != nil {
		return err
	}

	if cfg.NamedPort != "" {
		err = roxyutil.ValidateNamedPort(cfg.NamedPort)
		if err != nil {
			return err
		}
	}

	if cfg.Format != announcer.GRPCFormat {
		cfg.NamedPort = ""
	}

	return nil
}

// AddTo adds this configuration to the provided Announcer.
func (cfg EtcdAnnounceConfig) AddTo(etcd *v3.Client, a *announcer.Announcer) error {
	impl, err := announcer.NewEtcd(etcd, cfg.Path, cfg.Hostname, cfg.NamedPort, cfg.Format)
	if err == nil {
		a.Add(impl)
	}
	return err
}
