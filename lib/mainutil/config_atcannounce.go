package mainutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/announcer"
	"github.com/chronos-tachyon/roxy/lib/atcclient"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

// ATCAnnounceConfig represents the configuration for an *atcclient.ATCClient,
// plus the fields needed to call announcer.NewATC.
type ATCAnnounceConfig struct {
	ATCClientConfig
	ServiceName string
	Location    string
	Unique      string
	NamedPort   string
}

// ATCAnnounceConfigJSON represents the JSON doppelgänger of an ATCAnnounceConfig.
type ATCAnnounceConfigJSON struct {
	ATCClientConfigJSON
	ServiceName string `json:"serviceName"`
	Location    string `json:"location"`
	Unique      string `json:"unique"`
	NamedPort   string `json:"namedPort"`
}

// AppendTo appends the string representation to the given Builder.
func (cfg ATCAnnounceConfig) AppendTo(out *strings.Builder) {
	if !cfg.Enabled {
		return
	}
	cfg.ATCClientConfig.AppendTo(out)
	out.WriteString(";name=")
	out.WriteString(cfg.ServiceName)
	out.WriteString(";location=")
	out.WriteString(cfg.Location)
	out.WriteString(";unique=")
	out.WriteString(cfg.Unique)
	if cfg.NamedPort != "" {
		out.WriteString(";port=")
		out.WriteString(cfg.NamedPort)
	}
}

// String returns the string representation.
func (cfg ATCAnnounceConfig) String() string {
	if !cfg.Enabled {
		return ""
	}

	var buf strings.Builder
	buf.Grow(64)
	cfg.AppendTo(&buf)
	return buf.String()
}

// MarshalJSON fulfills json.Marshaler.
func (cfg ATCAnnounceConfig) MarshalJSON() ([]byte, error) {
	if !cfg.Enabled {
		return constants.NullBytes, nil
	}
	return json.Marshal(cfg.ToJSON())
}

// ToJSON converts the object to its JSON doppelgänger.
func (cfg ATCAnnounceConfig) ToJSON() *ATCAnnounceConfigJSON {
	if !cfg.Enabled {
		return nil
	}
	return &ATCAnnounceConfigJSON{
		ATCClientConfigJSON: *cfg.ATCClientConfig.ToJSON(),
		ServiceName:         cfg.ServiceName,
		Location:            cfg.Location,
		Unique:              cfg.Unique,
		NamedPort:           cfg.NamedPort,
	}
}

// Parse parses the string representation.
func (cfg *ATCAnnounceConfig) Parse(str string) error {
	if cfg == nil {
		panic(errors.New("*ATCAnnounceConfig is nil"))
	}

	wantZero := true
	defer func() {
		if wantZero {
			*cfg = ATCAnnounceConfig{}
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
		switch {
		case strings.HasPrefix(item, "name="):
			cfg.ServiceName, err = roxyutil.ExpandString(item[5:])
			if err != nil {
				return err
			}

		case strings.HasPrefix(item, "loc="):
			cfg.Location, err = roxyutil.ExpandString(item[4:])
			if err != nil {
				return err
			}

		case strings.HasPrefix(item, "location="):
			cfg.Location, err = roxyutil.ExpandString(item[9:])
			if err != nil {
				return err
			}

		case strings.HasPrefix(item, "unique="):
			cfg.Unique, err = roxyutil.ExpandString(item[7:])
			if err != nil {
				return err
			}

		case strings.HasPrefix(item, "port="):
			cfg.NamedPort, err = roxyutil.ExpandString(item[5:])
			if err != nil {
				return err
			}

		default:
			rest.WriteString(";")
			rest.WriteString(item)
		}
	}

	err = cfg.ATCClientConfig.Parse(rest.String())
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
func (cfg *ATCAnnounceConfig) UnmarshalJSON(raw []byte) error {
	if cfg == nil {
		panic(errors.New("*ATCAnnounceConfig is nil"))
	}

	wantZero := true
	defer func() {
		if wantZero {
			*cfg = ATCAnnounceConfig{}
		}
	}()

	if bytes.Equal(raw, constants.NullBytes) {
		return nil
	}

	var alt *ATCAnnounceConfigJSON
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
func (cfg *ATCAnnounceConfig) FromJSON(alt *ATCAnnounceConfigJSON) error {
	if cfg == nil {
		panic(errors.New("*ATCAnnounceConfig is nil"))
	}

	if alt == nil {
		*cfg = ATCAnnounceConfig{}
		return nil
	}

	*cfg = ATCAnnounceConfig{
		ServiceName: alt.ServiceName,
		Location:    alt.Location,
		Unique:      alt.Unique,
		NamedPort:   alt.NamedPort,
	}

	err := cfg.ATCClientConfig.FromJSON(&alt.ATCClientConfigJSON)
	if err != nil {
		return err
	}

	return nil
}

// PostProcess performs data integrity checks and input post-processing.
func (cfg *ATCAnnounceConfig) PostProcess() error {
	if cfg == nil {
		panic(errors.New("*ATCAnnounceConfig is nil"))
	}

	if !cfg.Enabled {
		*cfg = ATCAnnounceConfig{}
		return nil
	}

	err := cfg.ATCClientConfig.PostProcess()
	if err != nil {
		return err
	}

	err = roxyutil.ValidateATCServiceName(cfg.ServiceName)
	if err != nil {
		return err
	}

	err = roxyutil.ValidateATCLocation(cfg.Location)
	if err != nil {
		return err
	}

	if cfg.Unique == "" {
		cfg.Unique, err = UniqueID()
		if err != nil {
			return err
		}
	}
	err = roxyutil.ValidateATCUnique(cfg.Unique)
	if err != nil {
		return err
	}

	if cfg.NamedPort != "" {
		err = roxyutil.ValidateNamedPort(cfg.NamedPort)
		if err != nil {
			return err
		}
	}

	return nil
}

// AddTo adds this configuration to the provided Announcer.
func (cfg ATCAnnounceConfig) AddTo(client *atcclient.ATCClient, a *announcer.Announcer) error {
	impl, err := announcer.NewATC(client, cfg.ServiceName, cfg.Location, cfg.Unique, cfg.NamedPort)
	if err == nil {
		a.Add(impl)
	}
	return err
}
