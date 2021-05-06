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

type ZKAnnounceConfig struct {
	ZKConfig
	Path      string
	Format    announcer.Format
	NamedPort string
}

type zanncJSON struct {
	zcJSON
	Path      string           `json:"path"`
	Format    announcer.Format `json:"format"`
	NamedPort string           `json:"namedPort"`
}

func (zannc ZKAnnounceConfig) AppendTo(out *strings.Builder) {
	zannc.ZKConfig.AppendTo(out)
	out.WriteString(";path=")
	out.WriteString(zannc.Path)
	out.WriteString(";format=")
	out.WriteString(zannc.Format.String())
	if zannc.NamedPort != "" {
		out.WriteString(";port=")
		out.WriteString(zannc.NamedPort)
	}
}

func (zannc ZKAnnounceConfig) String() string {
	if !zannc.Enabled {
		return ""
	}

	var buf strings.Builder
	buf.Grow(64)
	zannc.AppendTo(&buf)
	return buf.String()
}

func (zannc ZKAnnounceConfig) MarshalJSON() ([]byte, error) {
	if !zannc.Enabled {
		return nullBytes, nil
	}
	return json.Marshal(zannc.toAlt())
}

func (zannc *ZKAnnounceConfig) Parse(str string) error {
	wantZero := true
	defer func() {
		if wantZero {
			*zannc = ZKAnnounceConfig{}
		}
	}()

	if str == "" || str == nullString {
		return nil
	}

	err := json.Unmarshal([]byte(str), zannc)
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
			zannc.Path = item[5:]

		case strings.HasPrefix(item, "format="):
			err = zannc.Format.Parse(item[7:])
			if err != nil {
				return fmt.Errorf("failed to parse format: %w", err)
			}

		case strings.HasPrefix(item, "port="):
			zannc.NamedPort = item[5:]

		default:
			rest.WriteString(";")
			rest.WriteString(item)
		}
	}

	err = zannc.ZKConfig.Parse(rest.String())
	if err != nil {
		return err
	}

	tmp, err := zannc.postprocess()
	if err != nil {
		return err
	}

	*zannc = tmp
	wantZero = false
	return nil
}

func (zannc *ZKAnnounceConfig) UnmarshalJSON(raw []byte) error {
	wantZero := true
	defer func() {
		if wantZero {
			*zannc = ZKAnnounceConfig{}
		}
	}()

	if bytes.Equal(raw, nullBytes) {
		return nil
	}

	var alt zanncJSON
	err := json.Unmarshal(raw, &alt)
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

	*zannc = tmp2
	wantZero = false
	return nil
}

func (zannc ZKAnnounceConfig) toAlt() *zanncJSON {
	if !zannc.Enabled {
		return nil
	}
	return &zanncJSON{
		zcJSON:    *zannc.ZKConfig.toAlt(),
		Path:      zannc.Path,
		Format:    zannc.Format,
		NamedPort: zannc.NamedPort,
	}
}

func (alt *zanncJSON) toStd() (ZKAnnounceConfig, error) {
	if alt == nil {
		return ZKAnnounceConfig{}, nil
	}
	tmp, err := alt.zcJSON.toStd()
	if err != nil {
		return ZKAnnounceConfig{}, err
	}
	return ZKAnnounceConfig{
		ZKConfig:  tmp,
		Path:      alt.Path,
		Format:    alt.Format,
		NamedPort: alt.NamedPort,
	}, nil
}

func (zannc ZKAnnounceConfig) postprocess() (out ZKAnnounceConfig, err error) {
	defer func() {
		log.Logger.Trace().
			Interface("result", out).
			Msg("ZKAnnounceConfig parse result")
	}()

	var zero ZKAnnounceConfig

	tmp, err := zannc.ZKConfig.postprocess()
	if err != nil {
		return zero, nil
	}
	zannc.ZKConfig = tmp

	if !strings.HasSuffix(zannc.Path, "/") {
		zannc.Path += "/"
	}
	err = roxyresolver.ValidateZKPath(zannc.Path)
	if err != nil {
		return zero, err
	}

	if zannc.NamedPort != "" {
		err = roxyresolver.ValidateServerSetPort(zannc.NamedPort)
		if err != nil {
			return zero, err
		}
	}

	if zannc.Format == announcer.FinagleFormat {
		zannc.NamedPort = ""
	}

	return zannc, nil
}
