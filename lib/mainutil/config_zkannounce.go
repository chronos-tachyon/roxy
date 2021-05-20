package mainutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	"github.com/go-zookeeper/zk"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/announcer"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

// ZKAnnounceConfig represents the configuration for a *zk.Conn, plus the
// fields needed to call announcer.NewZK.
type ZKAnnounceConfig struct {
	ZKConfig
	Path      string
	Unique    string
	NamedPort string
	Format    announcer.Format
}

// ZKAnnounceConfigJSON represents the JSON doppelgänger of an ZKAnnounceConfig.
type ZKAnnounceConfigJSON struct {
	ZKConfigJSON
	Path      string           `json:"path"`
	Unique    string           `json:"unique"`
	NamedPort string           `json:"namedPort"`
	Format    announcer.Format `json:"format"`
}

// AppendTo appends the string representation to the given Builder.
func (cfg ZKAnnounceConfig) AppendTo(out *strings.Builder) {
	if !cfg.Enabled {
		return
	}
	cfg.ZKConfig.AppendTo(out)
	out.WriteString(";path=")
	out.WriteString(cfg.Path)
	if cfg.Unique != "" {
		out.WriteString(";unique=")
		out.WriteString(cfg.Unique)
	}
	if cfg.NamedPort != "" {
		out.WriteString(";port=")
		out.WriteString(cfg.NamedPort)
	}
	out.WriteString(";format=")
	out.WriteString(cfg.Format.String())
}

// String returns the string representation.
func (cfg ZKAnnounceConfig) String() string {
	if !cfg.Enabled {
		return ""
	}

	var buf strings.Builder
	buf.Grow(64)
	cfg.AppendTo(&buf)
	return buf.String()
}

// MarshalJSON fulfills json.Marshaler.
func (cfg ZKAnnounceConfig) MarshalJSON() ([]byte, error) {
	if !cfg.Enabled {
		return constants.NullBytes, nil
	}
	return json.Marshal(cfg.ToJSON())
}

// ToJSON converts the object to its JSON doppelgänger.
func (cfg ZKAnnounceConfig) ToJSON() *ZKAnnounceConfigJSON {
	if !cfg.Enabled {
		return nil
	}
	return &ZKAnnounceConfigJSON{
		ZKConfigJSON: *cfg.ZKConfig.ToJSON(),
		Path:         cfg.Path,
		Format:       cfg.Format,
		NamedPort:    cfg.NamedPort,
	}
}

// Parse parses the string representation.
func (cfg *ZKAnnounceConfig) Parse(str string) error {
	if cfg == nil {
		panic(errors.New("*ZKAnnounceConfig is nil"))
	}

	wantZero := true
	defer func() {
		if wantZero {
			*cfg = ZKAnnounceConfig{}
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

		case optionUnique:
			cfg.Unique, err = roxyutil.ExpandString(optValue)
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

	err = cfg.ZKConfig.Parse(rest.String())
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
func (cfg *ZKAnnounceConfig) UnmarshalJSON(raw []byte) error {
	if cfg == nil {
		panic(errors.New("*ZKAnnounceConfig is nil"))
	}

	wantZero := true
	defer func() {
		if wantZero {
			*cfg = ZKAnnounceConfig{}
		}
	}()

	if bytes.Equal(raw, constants.NullBytes) {
		return nil
	}

	var alt *ZKAnnounceConfigJSON
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
func (cfg *ZKAnnounceConfig) FromJSON(alt *ZKAnnounceConfigJSON) error {
	if cfg == nil {
		panic(errors.New("*ZKAnnounceConfig is nil"))
	}

	if alt == nil {
		*cfg = ZKAnnounceConfig{}
		return nil
	}

	*cfg = ZKAnnounceConfig{
		Path:      alt.Path,
		Format:    alt.Format,
		NamedPort: alt.NamedPort,
	}

	err := cfg.ZKConfig.FromJSON(&alt.ZKConfigJSON)
	if err != nil {
		return err
	}

	return nil
}

// PostProcess performs data integrity checks and input post-processing.
func (cfg *ZKAnnounceConfig) PostProcess() error {
	if cfg == nil {
		panic(errors.New("*ZKAnnounceConfig is nil"))
	}

	if !cfg.Enabled {
		*cfg = ZKAnnounceConfig{}
		return nil
	}

	err := cfg.ZKConfig.PostProcess()
	if err != nil {
		return err
	}

	if !strings.HasPrefix(cfg.Path, "/") {
		cfg.Path = "/" + cfg.Path
	}
	err = roxyutil.ValidateZKPath(cfg.Path)
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
func (cfg ZKAnnounceConfig) AddTo(zkconn *zk.Conn, a *announcer.Announcer) error {
	impl, err := announcer.NewZK(zkconn, cfg.Path, cfg.Unique, cfg.NamedPort, cfg.Format)
	if err == nil {
		a.Add(impl)
	}
	return err
}
