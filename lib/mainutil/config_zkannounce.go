package mainutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-zookeeper/zk"

	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/announcer"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

type ZKAnnounceConfig struct {
	ZKConfig
	Path      string
	Unique    string
	NamedPort string
	Format    announcer.Format
}

type zacJSON struct {
	zcJSON
	Path      string           `json:"path"`
	Unique    string           `json:"unique"`
	NamedPort string           `json:"namedPort"`
	Format    announcer.Format `json:"format"`
}

func (zac ZKAnnounceConfig) AppendTo(out *strings.Builder) {
	zac.ZKConfig.AppendTo(out)
	out.WriteString(";path=")
	out.WriteString(zac.Path)
	if zac.Unique != "" {
		out.WriteString(";unique=")
		out.WriteString(zac.Unique)
	}
	if zac.NamedPort != "" {
		out.WriteString(";port=")
		out.WriteString(zac.NamedPort)
	}
	out.WriteString(";format=")
	out.WriteString(zac.Format.String())
}

func (zac ZKAnnounceConfig) String() string {
	if !zac.Enabled {
		return ""
	}

	var buf strings.Builder
	buf.Grow(64)
	zac.AppendTo(&buf)
	return buf.String()
}

func (zac ZKAnnounceConfig) MarshalJSON() ([]byte, error) {
	if !zac.Enabled {
		return nullBytes, nil
	}
	return json.Marshal(zac.toAlt())
}

func (zac *ZKAnnounceConfig) Parse(str string) error {
	wantZero := true
	defer func() {
		if wantZero {
			*zac = ZKAnnounceConfig{}
		}
	}()

	if str == "" || str == nullString {
		return nil
	}

	err := misc.StrictUnmarshalJSON([]byte(str), zac)
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
			zac.Path, err = roxyutil.ExpandString(item[5:])
			if err != nil {
				return err
			}

		case strings.HasPrefix(item, "unique="):
			zac.Unique, err = roxyutil.ExpandString(item[7:])
			if err != nil {
				return err
			}

		case strings.HasPrefix(item, "port="):
			zac.NamedPort, err = roxyutil.ExpandString(item[5:])
			if err != nil {
				return err
			}

		case strings.HasPrefix(item, "format="):
			err = zac.Format.Parse(item[7:])
			if err != nil {
				return fmt.Errorf("failed to parse format: %w", err)
			}

		default:
			rest.WriteString(";")
			rest.WriteString(item)
		}
	}

	err = zac.ZKConfig.Parse(rest.String())
	if err != nil {
		return err
	}

	tmp, err := zac.postprocess()
	if err != nil {
		return err
	}

	*zac = tmp
	wantZero = false
	return nil
}

func (zac *ZKAnnounceConfig) UnmarshalJSON(raw []byte) error {
	wantZero := true
	defer func() {
		if wantZero {
			*zac = ZKAnnounceConfig{}
		}
	}()

	if bytes.Equal(raw, nullBytes) {
		return nil
	}

	var alt zacJSON
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

	*zac = tmp2
	wantZero = false
	return nil
}

func (zac ZKAnnounceConfig) AddTo(zkconn *zk.Conn, a *announcer.Announcer) error {
	impl, err := announcer.NewZK(zkconn, zac.Path, zac.Unique, zac.NamedPort, zac.Format)
	if err == nil {
		a.Add(impl)
	}
	return err
}

func (zac ZKAnnounceConfig) toAlt() *zacJSON {
	if !zac.Enabled {
		return nil
	}
	return &zacJSON{
		zcJSON:    *zac.ZKConfig.toAlt(),
		Path:      zac.Path,
		Format:    zac.Format,
		NamedPort: zac.NamedPort,
	}
}

func (alt *zacJSON) toStd() (ZKAnnounceConfig, error) {
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

func (zac ZKAnnounceConfig) postprocess() (out ZKAnnounceConfig, err error) {
	var zero ZKAnnounceConfig

	tmp, err := zac.ZKConfig.postprocess()
	if err != nil {
		return zero, nil
	}
	zac.ZKConfig = tmp

	if !strings.HasPrefix(zac.Path, "/") {
		zac.Path = "/" + zac.Path
	}
	err = roxyutil.ValidateZKPath(zac.Path)
	if err != nil {
		return zero, err
	}

	if zac.NamedPort != "" {
		err = roxyutil.ValidateNamedPort(zac.NamedPort)
		if err != nil {
			return zero, err
		}
	}

	if zac.Format != announcer.GRPCFormat {
		zac.NamedPort = ""
	}

	return zac, nil
}
