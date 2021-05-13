package mainutil

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

type TLSServerConfig struct {
	Enabled     bool
	Cert        string
	Key         string
	MutualTLS   bool
	ClientCA    string
	CommonNames []string
}

type tscJSON struct {
	Cert        string   `json:"cert,omitempty"`
	Key         string   `json:"key,omitempty"`
	MutualTLS   bool     `json:"mTLS,omitempty"`
	ClientCA    string   `json:"clientCA,omitempty"`
	CommonNames []string `json:"commonNames,omitempty"`
}

func (tsc TLSServerConfig) AppendTo(out *strings.Builder) {
	if !tsc.Enabled {
		out.WriteString("no")
		return
	}
	out.WriteString("yes")
	out.WriteString(",cert=")
	out.WriteString(tsc.Cert)
	if tsc.Key != "" && tsc.Key != tsc.Cert {
		out.WriteString(",key=")
		out.WriteString(tsc.Key)
	}
	if tsc.MutualTLS {
		out.WriteString(",mtls=yes")
		if tsc.ClientCA != "" {
			out.WriteString(",ca=")
			out.WriteString(tsc.ClientCA)
		}
		if tsc.CommonNames != nil {
			out.WriteString(",cn=")
			appendCommonNameList(out, tsc.CommonNames)
		}
	}
}

func (tsc TLSServerConfig) String() string {
	if !tsc.Enabled {
		return "no"
	}

	var buf strings.Builder
	buf.Grow(64)
	tsc.AppendTo(&buf)
	return buf.String()
}

func (tsc TLSServerConfig) MarshalJSON() ([]byte, error) {
	if !tsc.Enabled {
		return nullBytes, nil
	}
	return json.Marshal(tsc.toAlt())
}

func (tsc *TLSServerConfig) Parse(str string) error {
	wantZero := true
	defer func() {
		if wantZero {
			*tsc = TLSServerConfig{}
		}
	}()

	if str == "" || str == nullString {
		return nil
	}

	err := misc.StrictUnmarshalJSON([]byte(str), tsc)
	if err == nil {
		wantZero = false
		return nil
	}

	pieces := strings.Split(str, ",")

	value, err := misc.ParseBool(pieces[0])
	if err != nil {
		return err
	}
	if !value {
		return nil
	}

	for _, item := range pieces[1:] {
		switch {
		case strings.HasPrefix(item, "cert="):
			tsc.Cert = item[5:]

		case strings.HasPrefix(item, "key="):
			tsc.Key = item[4:]

		case strings.HasPrefix(item, "mTLS=") || strings.HasPrefix(item, "mtls="):
			value, err = misc.ParseBool(item[5:])
			if err != nil {
				return err
			}
			tsc.MutualTLS = value

		case strings.HasPrefix(item, "clientCA="):
			tsc.ClientCA = item[9:]

		case strings.HasPrefix(item, "ca="):
			tsc.ClientCA = item[3:]

		case strings.HasPrefix(item, "commonNames="):
			tsc.CommonNames = parseCommonNameList(item[12:])

		case strings.HasPrefix(item, "cn="):
			tsc.CommonNames = parseCommonNameList(item[3:])

		default:
			return fmt.Errorf("unknown option %q", item)
		}
	}

	tsc.Enabled = true
	tmp, err := tsc.postprocess()
	if err != nil {
		return err
	}

	*tsc = tmp
	wantZero = false
	return nil
}

func (tsc *TLSServerConfig) UnmarshalJSON(raw []byte) error {
	wantZero := true
	defer func() {
		if wantZero {
			*tsc = TLSServerConfig{}
		}
	}()

	if bytes.Equal(raw, nullBytes) {
		return nil
	}

	var alt tscJSON
	err := misc.StrictUnmarshalJSON(raw, &alt)
	if err != nil {
		return err
	}

	tmp, err := alt.toStd().postprocess()
	if err != nil {
		return err
	}

	*tsc = tmp
	wantZero = false
	return nil
}

func (tsc TLSServerConfig) MakeTLS() (*tls.Config, error) {
	if !tsc.Enabled {
		return nil, nil
	}

	out := new(tls.Config)

	certPath := tsc.Cert
	keyPath := tsc.Key

	var err error
	out.Certificates = make([]tls.Certificate, 1)
	out.Certificates[0], err = tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load X.509 keypair from cert=%q key=%q: %w", certPath, keyPath, err)
	}

	if tsc.MutualTLS {
		out.ClientAuth = tls.RequireAndVerifyClientCert

		if tsc.ClientCA == "" {
			roots, err := x509.SystemCertPool()
			if err != nil {
				return nil, fmt.Errorf("failed to load system certificate pool: %w", err)
			}

			out.ClientCAs = roots
		} else {
			roots := x509.NewCertPool()

			raw, err := ioutil.ReadFile(tsc.ClientCA)
			if err != nil {
				return nil, fmt.Errorf("failed to read file %q: %w", tsc.ClientCA, err)
			}

			ok := roots.AppendCertsFromPEM(raw)
			if !ok {
				return nil, fmt.Errorf("failed to process certificates from PEM file %q", tsc.ClientCA)
			}

			out.ClientCAs = roots
		}

		out.VerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			if tsc.CommonNames == nil {
				return nil
			}
			actualCN := verifiedChains[0][0].Subject.CommonName
			for _, cn := range tsc.CommonNames {
				if actualCN == cn {
					return nil
				}
			}
			return fmt.Errorf("expected subject CommonName to be one of %q, got subject CommonName %q", tsc.CommonNames, actualCN)
		}
	}

	out.NextProtos = []string{"h2", "http/1.1"}
	return out, nil
}

func (tsc TLSServerConfig) toAlt() *tscJSON {
	if !tsc.Enabled {
		return nil
	}
	return &tscJSON{
		Cert:        tsc.Cert,
		Key:         tsc.Key,
		MutualTLS:   tsc.MutualTLS,
		ClientCA:    tsc.ClientCA,
		CommonNames: tsc.CommonNames,
	}
}

func (alt *tscJSON) toStd() TLSServerConfig {
	if alt == nil {
		return TLSServerConfig{}
	}
	return TLSServerConfig{
		Enabled:     true,
		Cert:        alt.Cert,
		Key:         alt.Key,
		MutualTLS:   alt.MutualTLS,
		ClientCA:    alt.ClientCA,
		CommonNames: alt.CommonNames,
	}
}

func (tsc TLSServerConfig) postprocess() (out TLSServerConfig, err error) {
	var zero TLSServerConfig

	if !tsc.Enabled {
		return zero, nil
	}

	if tsc.Cert == "" {
		return zero, nil
	}

	expanded, err := roxyutil.ExpandPath(tsc.Cert)
	if err != nil {
		return zero, err
	}
	tsc.Cert = expanded

	if tsc.Key != "" {
		expanded, err = roxyutil.ExpandPath(tsc.Key)
		if err != nil {
			return zero, err
		}
		tsc.Key = expanded
	}

	if tsc.Key == "" {
		tsc.Key = tsc.Cert
	}

	if !tsc.MutualTLS {
		tsc.ClientCA = ""
		tsc.CommonNames = nil
	}

	if tsc.ClientCA != "" {
		expanded, err = roxyutil.ExpandPath(tsc.ClientCA)
		if err != nil {
			return zero, err
		}
		tsc.ClientCA = expanded
	}

	return tsc, nil
}

func parseCommonNameList(str string) []string {
	pieces := strings.Split(str, ":")
	out := make([]string, 0, len(pieces))
	for _, item := range pieces {
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func appendCommonNameList(out *strings.Builder, list []string) {
	if len(list) == 0 {
		out.WriteString(":")
		return
	}
	for i, value := range list {
		if i != 0 {
			out.WriteString(":")
		}
		out.WriteString(value)
	}
}
