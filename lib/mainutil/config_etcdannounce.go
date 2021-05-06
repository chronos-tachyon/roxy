package mainutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/chronos-tachyon/roxy/lib/announcer"
	"github.com/chronos-tachyon/roxy/lib/roxyresolver"
)

type EtcdAnnounceConfig struct {
	EtcdConfig
	Path      string
	Format    announcer.Format
	NamedPort string
}

type eacJSON struct {
	ecJSON
	Path      string           `json:"path"`
	Format    announcer.Format `json:"format"`
	NamedPort string           `json:"namedPort"`
}

func (eac EtcdAnnounceConfig) AppendTo(out *strings.Builder) {
	eac.EtcdConfig.AppendTo(out)
	out.WriteString(";path=")
	out.WriteString(eac.Path)
	out.WriteString(";format=")
	out.WriteString(eac.Format.String())
	if eac.NamedPort != "" {
		out.WriteString(";port=")
		out.WriteString(eac.NamedPort)
	}
}

func (eac EtcdAnnounceConfig) String() string {
	if !eac.Enabled {
		return ""
	}

	var buf strings.Builder
	buf.Grow(64)
	eac.AppendTo(&buf)
	return buf.String()
}

func (eac EtcdAnnounceConfig) MarshalJSON() ([]byte, error) {
	if !eac.Enabled {
		return nullBytes, nil
	}
	return json.Marshal(eac.toAlt())
}

func (eac *EtcdAnnounceConfig) Parse(str string) error {
	wantZero := true
	defer func() {
		if wantZero {
			*eac = EtcdAnnounceConfig{}
		}
	}()

	if str == "" || str == nullString {
		return nil
	}

	err := json.Unmarshal([]byte(str), eac)
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
		case strings.HasPrefix(item, "path="):
			eac.Path = item[5:]

		case strings.HasPrefix(item, "format="):
			err = eac.Format.Parse(item[7:])
			if err != nil {
				return fmt.Errorf("failed to parse format: %w", err)
			}

		case strings.HasPrefix(item, "port="):
			eac.NamedPort = item[5:]

		default:
			rest.WriteString(";")
			rest.WriteString(item)
		}
	}

	err = eac.EtcdConfig.Parse(rest.String())
	if err != nil {
		return err
	}

	tmp, err := eac.postprocess()
	if err != nil {
		return err
	}

	*eac = tmp
	wantZero = false
	return nil
}

func (eac *EtcdAnnounceConfig) UnmarshalJSON(raw []byte) error {
	wantZero := true
	defer func() {
		if wantZero {
			*eac = EtcdAnnounceConfig{}
		}
	}()

	if bytes.Equal(raw, nullBytes) {
		return nil
	}

	var alt eacJSON
	err := json.Unmarshal(raw, &alt)
	if err != nil {
		return err
	}

	tmp, err := alt.toStd().postprocess()
	if err != nil {
		return err
	}

	*eac = tmp
	wantZero = false
	return nil
}

func (eac EtcdAnnounceConfig) toAlt() *eacJSON {
	if !eac.Enabled {
		return nil
	}
	return &eacJSON{
		ecJSON:    *eac.EtcdConfig.toAlt(),
		Path:      eac.Path,
		Format:    eac.Format,
		NamedPort: eac.NamedPort,
	}
}

func (alt *eacJSON) toStd() EtcdAnnounceConfig {
	if alt == nil {
		return EtcdAnnounceConfig{}
	}
	return EtcdAnnounceConfig{
		EtcdConfig: alt.ecJSON.toStd(),
		Path:       alt.Path,
		Format:     alt.Format,
		NamedPort:  alt.NamedPort,
	}
}

func (eac EtcdAnnounceConfig) postprocess() (out EtcdAnnounceConfig, err error) {
	defer func() {
		log.Logger.Trace().
			Interface("result", out).
			Msg("EtcdAnnounceConfig parse result")
	}()

	var zero EtcdAnnounceConfig

	tmp, err := eac.EtcdConfig.postprocess()
	if err != nil {
		return zero, nil
	}
	eac.EtcdConfig = tmp

	if !strings.HasSuffix(eac.Path, "/") {
		eac.Path += "/"
	}
	err = roxyresolver.ValidateEtcdPath(eac.Path)
	if err != nil {
		return zero, err
	}

	if eac.NamedPort != "" {
		err = roxyresolver.ValidateServerSetPort(eac.NamedPort)
		if err != nil {
			return zero, err
		}
	}

	if eac.Format == announcer.FinagleFormat {
		eac.NamedPort = ""
	}

	return eac, nil
}
