package mainutil

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/announcer"
	"github.com/chronos-tachyon/roxy/lib/atcclient"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

type ATCAnnounceConfig struct {
	ATCClientConfig
	ServiceName string
	Location    string
	Unique      string
	NamedPort   string
}

type aacJSON struct {
	accJSON
	ServiceName string `json:"serviceName"`
	Location    string `json:"location"`
	Unique      string `json:"unique"`
	NamedPort   string `json:"namedPort"`
}

func (aac ATCAnnounceConfig) AppendTo(out *strings.Builder) {
	aac.ATCClientConfig.AppendTo(out)
	out.WriteString(";name=")
	out.WriteString(aac.ServiceName)
	out.WriteString(";location=")
	out.WriteString(aac.Location)
	out.WriteString(";unique=")
	out.WriteString(aac.Unique)
	if aac.NamedPort != "" {
		out.WriteString(";port=")
		out.WriteString(aac.NamedPort)
	}
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
		case strings.HasPrefix(item, "name="):
			aac.ServiceName, err = roxyutil.ExpandString(item[5:])
			if err != nil {
				return err
			}

		case strings.HasPrefix(item, "loc="):
			aac.Location, err = roxyutil.ExpandString(item[4:])
			if err != nil {
				return err
			}

		case strings.HasPrefix(item, "location="):
			aac.Location, err = roxyutil.ExpandString(item[9:])
			if err != nil {
				return err
			}

		case strings.HasPrefix(item, "unique="):
			aac.Unique, err = roxyutil.ExpandString(item[7:])
			if err != nil {
				return err
			}

		case strings.HasPrefix(item, "port="):
			aac.NamedPort, err = roxyutil.ExpandString(item[5:])
			if err != nil {
				return err
			}

		default:
			rest.WriteString(";")
			rest.WriteString(item)
		}
	}

	err = aac.ATCClientConfig.Parse(rest.String())
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

func (aac ATCAnnounceConfig) AddTo(client *atcclient.ATCClient, loadFn atcclient.LoadFunc, a *announcer.Announcer) error {
	impl, err := announcer.NewATC(client, aac.ServiceName, aac.Location, aac.Unique, aac.NamedPort, loadFn)
	if err == nil {
		a.Add(impl)
	}
	return err
}

func (aac ATCAnnounceConfig) toAlt() *aacJSON {
	if !aac.Enabled {
		return nil
	}
	return &aacJSON{
		accJSON:     *aac.ATCClientConfig.toAlt(),
		ServiceName: aac.ServiceName,
		Location:    aac.Location,
		Unique:      aac.Unique,
		NamedPort:   aac.NamedPort,
	}
}

func (alt *aacJSON) toStd() (ATCAnnounceConfig, error) {
	if alt == nil {
		return ATCAnnounceConfig{}, nil
	}
	tmp, err := alt.accJSON.toStd()
	if err != nil {
		return ATCAnnounceConfig{}, err
	}
	return ATCAnnounceConfig{
		ATCClientConfig: tmp,
		ServiceName:     alt.ServiceName,
		Location:        alt.Location,
		Unique:          alt.Unique,
		NamedPort:       alt.NamedPort,
	}, nil
}

func (aac ATCAnnounceConfig) postprocess() (out ATCAnnounceConfig, err error) {
	var zero ATCAnnounceConfig

	tmp, err := aac.ATCClientConfig.postprocess()
	if err != nil {
		return zero, nil
	}
	aac.ATCClientConfig = tmp

	err = roxyutil.ValidateATCServiceName(aac.ServiceName)
	if err != nil {
		return zero, err
	}

	err = roxyutil.ValidateATCLocation(aac.Location)
	if err != nil {
		return zero, err
	}

	if aac.Unique == "" {
		aac.Unique, err = UniqueID()
		if err != nil {
			return zero, err
		}
	}
	err = roxyutil.ValidateATCUnique(aac.Unique)
	if err != nil {
		return zero, err
	}

	if aac.NamedPort != "" {
		err = roxyutil.ValidateNamedPort(aac.NamedPort)
		if err != nil {
			return zero, err
		}
	}

	return aac, nil
}
