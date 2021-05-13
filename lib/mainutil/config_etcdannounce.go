package mainutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	v3 "go.etcd.io/etcd/client/v3"

	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/announcer"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

type EtcdAnnounceConfig struct {
	EtcdConfig
	Path      string
	Unique    string
	NamedPort string
	Format    announcer.Format
}

type eacJSON struct {
	ecJSON
	Path      string           `json:"path"`
	Unique    string           `json:"unique"`
	NamedPort string           `json:"namedPort"`
	Format    announcer.Format `json:"format"`
}

func (eac EtcdAnnounceConfig) AppendTo(out *strings.Builder) {
	eac.EtcdConfig.AppendTo(out)
	out.WriteString(";path=")
	out.WriteString(eac.Path)
	if eac.Unique != "" {
		out.WriteString(";unique=")
		out.WriteString(eac.Unique)
	}
	if eac.NamedPort != "" {
		out.WriteString(";port=")
		out.WriteString(eac.NamedPort)
	}
	out.WriteString(";format=")
	out.WriteString(eac.Format.String())
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

	err := misc.StrictUnmarshalJSON([]byte(str), eac)
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
			eac.Path, err = roxyutil.ExpandString(item[5:])
			if err != nil {
				return err
			}

		case strings.HasPrefix(item, "unique="):
			eac.Unique, err = roxyutil.ExpandString(item[7:])
			if err != nil {
				return err
			}

		case strings.HasPrefix(item, "port="):
			eac.NamedPort, err = roxyutil.ExpandString(item[5:])
			if err != nil {
				return err
			}

		case strings.HasPrefix(item, "format="):
			err = eac.Format.Parse(item[7:])
			if err != nil {
				return fmt.Errorf("failed to parse format: %w", err)
			}

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
	err := misc.StrictUnmarshalJSON(raw, &alt)
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

func (eac EtcdAnnounceConfig) AddTo(etcd *v3.Client, a *announcer.Announcer) error {
	impl, err := announcer.NewEtcd(etcd, eac.Path, eac.Unique, eac.NamedPort, eac.Format)
	if err == nil {
		a.Add(impl)
	}
	return err
}

func (eac EtcdAnnounceConfig) toAlt() *eacJSON {
	if !eac.Enabled {
		return nil
	}
	return &eacJSON{
		ecJSON:    *eac.EtcdConfig.toAlt(),
		Path:      eac.Path,
		NamedPort: eac.NamedPort,
		Format:    eac.Format,
	}
}

func (alt *eacJSON) toStd() EtcdAnnounceConfig {
	if alt == nil {
		return EtcdAnnounceConfig{}
	}
	return EtcdAnnounceConfig{
		EtcdConfig: alt.ecJSON.toStd(),
		Path:       alt.Path,
		NamedPort:  alt.NamedPort,
		Format:     alt.Format,
	}
}

func (eac EtcdAnnounceConfig) postprocess() (out EtcdAnnounceConfig, err error) {
	var zero EtcdAnnounceConfig

	tmp, err := eac.EtcdConfig.postprocess()
	if err != nil {
		return zero, nil
	}
	eac.EtcdConfig = tmp

	if !strings.HasSuffix(eac.Path, "/") {
		eac.Path += "/"
	}
	err = roxyutil.ValidateEtcdPath(eac.Path)
	if err != nil {
		return zero, err
	}

	if eac.NamedPort != "" {
		err = roxyutil.ValidateNamedPort(eac.NamedPort)
		if err != nil {
			return zero, err
		}
	}

	if eac.Format != announcer.GRPCFormat {
		eac.NamedPort = ""
	}

	return eac, nil
}
