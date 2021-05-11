package mainutil

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

type TLSClientConfig struct {
	Enabled              bool
	SkipVerify           bool
	SkipVerifyServerName bool
	RootCA               string
	ServerName           string
	CommonName           string
	ClientCert           string
	ClientKey            string
}

type tccJSON struct {
	SkipVerify           bool   `json:"skipVerify,omitempty"`
	SkipVerifyServerName bool   `json:"skipVerifyServerName,omitempty"`
	RootCA               string `json:"rootCA,omitempty"`
	ServerName           string `json:"serverName,omitempty"`
	CommonName           string `json:"commonName,omitempty"`
	ClientCert           string `json:"clientCert,omitempty"`
	ClientKey            string `json:"clientKey,omitempty"`
}

func (tcc TLSClientConfig) AppendTo(out *strings.Builder) {
	if !tcc.Enabled {
		out.WriteString("no")
		return
	}
	out.WriteString("yes")
	if tcc.SkipVerify {
		out.WriteString(",verify=no")
	}
	if tcc.SkipVerifyServerName {
		out.WriteString(",verifyServerName=no")
	}
	if tcc.RootCA != "" {
		out.WriteString(",ca=")
		out.WriteString(tcc.RootCA)
	}
	if tcc.ServerName != "" {
		out.WriteString(",serverName=")
		out.WriteString(tcc.ServerName)
	}
	if tcc.CommonName != "" {
		out.WriteString(",commonName=")
		out.WriteString(tcc.CommonName)
	}
	if tcc.ClientCert != "" {
		out.WriteString(",clientCert=")
		out.WriteString(tcc.ClientCert)
	}
	if tcc.ClientKey != "" {
		out.WriteString(",clientKey=")
		out.WriteString(tcc.ClientKey)
	}
}

func (tcc TLSClientConfig) String() string {
	if !tcc.Enabled {
		return "no"
	}

	var buf strings.Builder
	buf.Grow(64)
	tcc.AppendTo(&buf)
	return buf.String()
}

func (tcc TLSClientConfig) MarshalJSON() ([]byte, error) {
	if !tcc.Enabled {
		return nullBytes, nil
	}
	return json.Marshal(tcc.toAlt())
}

func (tcc *TLSClientConfig) Parse(str string) error {
	wantZero := true
	defer func() {
		if wantZero {
			*tcc = TLSClientConfig{}
		}
	}()

	if str == "" || str == nullString {
		return nil
	}

	err := misc.StrictUnmarshalJSON([]byte(str), tcc)
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
		case strings.HasPrefix(item, "verify="):
			value, err = misc.ParseBool(item[7:])
			if err != nil {
				return err
			}
			tcc.SkipVerify = !value

		case strings.HasPrefix(item, "verifyServerName="):
			value, err = misc.ParseBool(item[17:])
			if err != nil {
				return err
			}
			tcc.SkipVerifyServerName = !value

		case strings.HasPrefix(item, "ca="):
			tcc.RootCA = item[3:]

		case strings.HasPrefix(item, "serverName="):
			tcc.ServerName = item[11:]

		case strings.HasPrefix(item, "commonName="):
			tcc.CommonName = item[11:]

		case strings.HasPrefix(item, "cn="):
			tcc.CommonName = item[3:]

		case strings.HasPrefix(item, "clientCert="):
			tcc.ClientCert = item[11:]

		case strings.HasPrefix(item, "clientKey="):
			tcc.ClientKey = item[10:]

		default:
			return fmt.Errorf("unknown option %q", item)
		}
	}

	tcc.Enabled = true
	tmp, err := tcc.postprocess()
	if err != nil {
		return err
	}

	*tcc = tmp
	wantZero = false
	return nil
}

func (tcc *TLSClientConfig) UnmarshalJSON(raw []byte) error {
	wantZero := true
	defer func() {
		if wantZero {
			*tcc = TLSClientConfig{}
		}
	}()

	if bytes.Equal(raw, nullBytes) {
		return nil
	}

	var alt tccJSON
	err := misc.StrictUnmarshalJSON(raw, &alt)
	if err != nil {
		return err
	}

	tmp, err := alt.toStd().postprocess()
	if err != nil {
		return err
	}

	*tcc = tmp
	wantZero = false
	return nil
}

func (tcc TLSClientConfig) MakeTLS(serverName string) (*tls.Config, error) {
	if !tcc.Enabled {
		return nil, nil
	}

	out := new(tls.Config)

	if tcc.SkipVerify {
		// pass
	} else if tcc.RootCA == "" {
		roots, err := x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("failed to load system certificate pool: %w", err)
		}

		out.RootCAs = roots
	} else {
		roots := x509.NewCertPool()

		raw, err := ioutil.ReadFile(tcc.RootCA)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %q: %w", tcc.RootCA, err)
		}

		ok := roots.AppendCertsFromPEM(raw)
		if !ok {
			return nil, fmt.Errorf("failed to process certificates from PEM file %q", tcc.RootCA)
		}

		out.RootCAs = roots
	}

	out.ServerName = tcc.ServerName
	if out.ServerName == "" {
		out.ServerName = serverName
	}

	if tcc.SkipVerify {
		out.InsecureSkipVerify = true
	} else if tcc.SkipVerifyServerName {
		out.InsecureSkipVerify = true
		out.VerifyConnection = func(cs tls.ConnectionState) error {
			opts := x509.VerifyOptions{
				Roots:         out.RootCAs,
				Intermediates: x509.NewCertPool(),
				DNSName:       "",
			}

			for _, cert := range cs.PeerCertificates[1:] {
				opts.Intermediates.AddCert(cert)
			}

			_, err := cs.PeerCertificates[0].Verify(opts)
			if err != nil {
				return err
			}

			if tcc.CommonName != "" {
				expectCN := tcc.CommonName
				actualCN := cs.PeerCertificates[0].Subject.CommonName
				if expectCN != actualCN {
					return fmt.Errorf("expected subject CommonName %q, got subject CommonName %q", expectCN, actualCN)
				}
			}

			return nil
		}
	} else if tcc.CommonName != "" {
		out.VerifyConnection = func(cs tls.ConnectionState) error {
			expectCN := tcc.CommonName
			actualCN := cs.PeerCertificates[0].Subject.CommonName
			if expectCN != actualCN {
				return fmt.Errorf("expected subject CommonName %q, got subject CommonName %q", expectCN, actualCN)
			}
			return nil
		}
	}

	if tcc.ClientCert != "" {
		certPath := tcc.ClientCert
		keyPath := tcc.ClientKey

		var err error
		out.Certificates = make([]tls.Certificate, 1)
		out.Certificates[0], err = tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load X.509 keypair from cert=%q key=%q: %w", certPath, keyPath, err)
		}
	}

	return out, nil
}

func (tcc TLSClientConfig) toAlt() *tccJSON {
	if !tcc.Enabled {
		return nil
	}
	return &tccJSON{
		SkipVerify:           tcc.SkipVerify,
		SkipVerifyServerName: tcc.SkipVerifyServerName,
		RootCA:               tcc.RootCA,
		ServerName:           tcc.ServerName,
		CommonName:           tcc.CommonName,
		ClientCert:           tcc.ClientCert,
		ClientKey:            tcc.ClientKey,
	}
}

func (alt *tccJSON) toStd() TLSClientConfig {
	if alt == nil {
		return TLSClientConfig{}
	}
	return TLSClientConfig{
		Enabled:              true,
		SkipVerify:           alt.SkipVerify,
		SkipVerifyServerName: alt.SkipVerifyServerName,
		RootCA:               alt.RootCA,
		ServerName:           alt.ServerName,
		CommonName:           alt.CommonName,
		ClientCert:           alt.ClientCert,
		ClientKey:            alt.ClientKey,
	}
}

func (tcc TLSClientConfig) postprocess() (out TLSClientConfig, err error) {
	defer func() {
		log.Logger.Trace().
			Interface("result", out).
			Msg("TLSClientConfig parse result")
	}()

	var zero TLSClientConfig

	if !tcc.Enabled {
		return zero, nil
	}

	if tcc.SkipVerify {
		tcc.RootCA = ""
		tcc.ServerName = ""
		tcc.CommonName = ""
		tcc.SkipVerifyServerName = false
	}

	if tcc.SkipVerifyServerName {
		tcc.ServerName = ""
	}

	if tcc.RootCA != "" {
		expanded, err := roxyutil.ExpandPath(tcc.RootCA)
		if err != nil {
			return zero, err
		}
		tcc.RootCA = expanded
	}

	if tcc.ClientCert != "" {
		expanded, err := roxyutil.ExpandPath(tcc.ClientCert)
		if err != nil {
			return zero, err
		}
		tcc.ClientCert = expanded
	}

	if tcc.ClientKey != "" {
		expanded, err := roxyutil.ExpandPath(tcc.ClientKey)
		if err != nil {
			return zero, err
		}
		tcc.ClientKey = expanded
	}

	if tcc.ClientCert != "" && tcc.ClientKey == "" {
		tcc.ClientKey = tcc.ClientCert
	}

	return tcc, nil
}
