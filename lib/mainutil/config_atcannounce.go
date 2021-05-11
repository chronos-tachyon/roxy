package mainutil

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

type ATCAnnounceConfig struct {
	GRPCClientConfig
	NamedPort   string
	ServiceName string
	Location    string
}

type aacJSON struct {
	gccJSON
	NamedPort   string `json:"namedPort"`
	ServiceName string `json:"serviceName"`
	Location    string `json:"location"`
}

func (aac ATCAnnounceConfig) AppendTo(out *strings.Builder) {
	aac.GRPCClientConfig.AppendTo(out)
	if aac.NamedPort != "" {
		out.WriteString(";port=")
		out.WriteString(aac.NamedPort)
	}
	out.WriteString(";name=")
	out.WriteString(aac.ServiceName)
	out.WriteString(";loc=")
	out.WriteString(aac.Location)
}

func (aac ATCAnnounceConfig) String() string {
	if !aac.Enabled {
		return ""
	}

	var buf strings.Builder
	buf.Grow(64)
	aac.AppendTo(&buf)
	return buf.String()
}

func (aac ATCAnnounceConfig) MarshalJSON() ([]byte, error) {
	if !aac.Enabled {
		return nullBytes, nil
	}
	return json.Marshal(aac.toAlt())
}

func (aac *ATCAnnounceConfig) Parse(str string) error {
	wantZero := true
	defer func() {
		if wantZero {
			*aac = ATCAnnounceConfig{}
		}
	}()

	if str == "" || str == nullString {
		return nil
	}

	err := misc.StrictUnmarshalJSON([]byte(str), aac)
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
		case strings.HasPrefix(item, "port="):
			aac.NamedPort = item[5:]

		case strings.HasPrefix(item, "name="):
			aac.ServiceName = item[5:]

		case strings.HasPrefix(item, "loc="):
			aac.Location = item[4:]

		default:
			rest.WriteString(";")
			rest.WriteString(item)
		}
	}

	err = aac.GRPCClientConfig.Parse(rest.String())
	if err != nil {
		return err
	}

	tmp, err := aac.postprocess()
	if err != nil {
		return err
	}

	*aac = tmp
	wantZero = false
	return nil
}

func (aac *ATCAnnounceConfig) UnmarshalJSON(raw []byte) error {
	wantZero := true
	defer func() {
		if wantZero {
			*aac = ATCAnnounceConfig{}
		}
	}()

	if bytes.Equal(raw, nullBytes) {
		return nil
	}

	var alt aacJSON
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

	*aac = tmp2
	wantZero = false
	return nil
}

func (aac ATCAnnounceConfig) toAlt() *aacJSON {
	if !aac.Enabled {
		return nil
	}
	return &aacJSON{
		gccJSON:     *aac.GRPCClientConfig.toAlt(),
		NamedPort:   aac.NamedPort,
		ServiceName: aac.ServiceName,
		Location:    aac.Location,
	}
}

func (alt *aacJSON) toStd() (ATCAnnounceConfig, error) {
	if alt == nil {
		return ATCAnnounceConfig{}, nil
	}
	tmp, err := alt.gccJSON.toStd()
	if err != nil {
		return ATCAnnounceConfig{}, err
	}
	return ATCAnnounceConfig{
		GRPCClientConfig: tmp,
		NamedPort:        alt.NamedPort,
		ServiceName:      alt.ServiceName,
		Location:         alt.Location,
	}, nil
}

func (aac ATCAnnounceConfig) postprocess() (out ATCAnnounceConfig, err error) {
	defer func() {
		log.Logger.Trace().
			Interface("result", out).
			Msg("ATCAnnounceConfig parse result")
	}()

	var zero ATCAnnounceConfig

	tmp, err := aac.GRPCClientConfig.postprocess()
	if err != nil {
		return zero, nil
	}
	aac.GRPCClientConfig = tmp

	if aac.NamedPort != "" {
		err = roxyutil.ValidateNamedPort(aac.NamedPort)
		if err != nil {
			return zero, err
		}
	}

	err = roxyutil.ValidateATCServiceName(aac.ServiceName)
	if err != nil {
		return zero, err
	}

	err = roxyutil.ValidateATCLocation(aac.Location)
	if err != nil {
		return zero, err
	}

	return aac, nil
}
