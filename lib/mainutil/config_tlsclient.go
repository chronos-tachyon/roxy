package mainutil

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

// TLSClientConfig represents the configuration for a client-oriented
// *tls.Config object.
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

// TLSClientConfigJSON represents the JSON doppelgänger of an TLSClientConfig.
type TLSClientConfigJSON struct {
	SkipVerify           bool   `json:"skipVerify,omitempty"`
	SkipVerifyServerName bool   `json:"skipVerifyServerName,omitempty"`
	RootCA               string `json:"rootCA,omitempty"`
	ServerName           string `json:"serverName,omitempty"`
	CommonName           string `json:"commonName,omitempty"`
	ClientCert           string `json:"clientCert,omitempty"`
	ClientKey            string `json:"clientKey,omitempty"`
}

// AppendTo appends the string representation to the given Builder.
func (cfg TLSClientConfig) AppendTo(out *strings.Builder) {
	if !cfg.Enabled {
		out.WriteString("no")
		return
	}
	out.WriteString("yes")
	if cfg.SkipVerify {
		out.WriteString(",verify=no")
	}
	if cfg.SkipVerifyServerName {
		out.WriteString(",verifyServerName=no")
	}
	if cfg.RootCA != "" {
		out.WriteString(",ca=")
		out.WriteString(cfg.RootCA)
	}
	if cfg.ServerName != "" {
		out.WriteString(",serverName=")
		out.WriteString(cfg.ServerName)
	}
	if cfg.CommonName != "" {
		out.WriteString(",commonName=")
		out.WriteString(cfg.CommonName)
	}
	if cfg.ClientCert != "" {
		out.WriteString(",clientCert=")
		out.WriteString(cfg.ClientCert)
	}
	if cfg.ClientKey != "" {
		out.WriteString(",clientKey=")
		out.WriteString(cfg.ClientKey)
	}
}

// String returns the string representation.
func (cfg TLSClientConfig) String() string {
	if !cfg.Enabled {
		return "no"
	}

	var buf strings.Builder
	buf.Grow(64)
	cfg.AppendTo(&buf)
	return buf.String()
}

// MarshalJSON fulfills json.Marshaler.
func (cfg TLSClientConfig) MarshalJSON() ([]byte, error) {
	if !cfg.Enabled {
		return constants.NullBytes, nil
	}
	return json.Marshal(cfg.ToJSON())
}

// ToJSON converts the object to its JSON doppelgänger.
func (cfg TLSClientConfig) ToJSON() *TLSClientConfigJSON {
	if !cfg.Enabled {
		return nil
	}
	return &TLSClientConfigJSON{
		SkipVerify:           cfg.SkipVerify,
		SkipVerifyServerName: cfg.SkipVerifyServerName,
		RootCA:               cfg.RootCA,
		ServerName:           cfg.ServerName,
		CommonName:           cfg.CommonName,
		ClientCert:           cfg.ClientCert,
		ClientKey:            cfg.ClientKey,
	}
}

// Parse parses the string representation.
func (cfg *TLSClientConfig) Parse(str string) error {
	if cfg == nil {
		panic(errors.New("*TLSClientConfig is nil"))
	}

	wantZero := true
	defer func() {
		if wantZero {
			*cfg = TLSClientConfig{}
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

	pieces := strings.Split(str, ",")

	value, err := misc.ParseBool(pieces[0])
	if err != nil {
		return err
	}
	if !value {
		return nil
	}

	cfg.Enabled = true

	for _, item := range pieces[1:] {
		switch {
		case strings.HasPrefix(item, "verify="):
			value, err = misc.ParseBool(item[7:])
			if err != nil {
				return err
			}
			cfg.SkipVerify = !value

		case strings.HasPrefix(item, "verifyServerName="):
			value, err = misc.ParseBool(item[17:])
			if err != nil {
				return err
			}
			cfg.SkipVerifyServerName = !value

		case strings.HasPrefix(item, "ca="):
			cfg.RootCA = item[3:]

		case strings.HasPrefix(item, "serverName="):
			cfg.ServerName = item[11:]

		case strings.HasPrefix(item, "commonName="):
			cfg.CommonName = item[11:]

		case strings.HasPrefix(item, "cn="):
			cfg.CommonName = item[3:]

		case strings.HasPrefix(item, "clientCert="):
			cfg.ClientCert = item[11:]

		case strings.HasPrefix(item, "clientKey="):
			cfg.ClientKey = item[10:]

		default:
			return fmt.Errorf("unknown option %q", item)
		}
	}

	err = cfg.PostProcess()
	if err != nil {
		return err
	}

	wantZero = false
	return nil
}

// UnmarshalJSON fulfills json.Unmarshaler.
func (cfg *TLSClientConfig) UnmarshalJSON(raw []byte) error {
	if cfg == nil {
		panic(errors.New("*TLSClientConfig is nil"))
	}

	wantZero := true
	defer func() {
		if wantZero {
			*cfg = TLSClientConfig{}
		}
	}()

	if bytes.Equal(raw, constants.NullBytes) {
		return nil
	}

	var alt *TLSClientConfigJSON
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
func (cfg *TLSClientConfig) FromJSON(alt *TLSClientConfigJSON) error {
	if cfg == nil {
		panic(errors.New("*TLSClientConfig is nil"))
	}

	if alt == nil {
		*cfg = TLSClientConfig{}
		return nil
	}

	*cfg = TLSClientConfig{
		Enabled:              true,
		SkipVerify:           alt.SkipVerify,
		SkipVerifyServerName: alt.SkipVerifyServerName,
		RootCA:               alt.RootCA,
		ServerName:           alt.ServerName,
		CommonName:           alt.CommonName,
		ClientCert:           alt.ClientCert,
		ClientKey:            alt.ClientKey,
	}

	return nil
}

// PostProcess performs data integrity checks and input post-processing.
func (cfg *TLSClientConfig) PostProcess() error {
	if cfg == nil {
		panic(errors.New("*TLSClientConfig is nil"))
	}

	if !cfg.Enabled {
		*cfg = TLSClientConfig{}
		return nil
	}

	if cfg.SkipVerify {
		cfg.RootCA = ""
		cfg.ServerName = ""
		cfg.CommonName = ""
		cfg.SkipVerifyServerName = false
	}

	if cfg.SkipVerifyServerName {
		cfg.ServerName = ""
	}

	if cfg.RootCA != "" {
		expanded, err := roxyutil.ExpandPath(cfg.RootCA)
		if err != nil {
			return err
		}
		cfg.RootCA = expanded
	}

	if cfg.ClientCert != "" {
		expanded, err := roxyutil.ExpandPath(cfg.ClientCert)
		if err != nil {
			return err
		}
		cfg.ClientCert = expanded
	}

	if cfg.ClientKey != "" {
		expanded, err := roxyutil.ExpandPath(cfg.ClientKey)
		if err != nil {
			return err
		}
		cfg.ClientKey = expanded
	}

	if cfg.ClientCert != "" && cfg.ClientKey == "" {
		cfg.ClientKey = cfg.ClientCert
	}

	return nil
}

// MakeTLS constructs the configured *tls.Config.
func (cfg TLSClientConfig) MakeTLS(serverName string) (*tls.Config, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	out := new(tls.Config)

	if cfg.SkipVerify {
		// pass
	} else if cfg.RootCA == "" {
		roots, err := x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("failed to load system certificate pool: %w", err)
		}

		out.RootCAs = roots
	} else {
		roots := x509.NewCertPool()

		raw, err := ioutil.ReadFile(cfg.RootCA)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %q: %w", cfg.RootCA, err)
		}

		ok := roots.AppendCertsFromPEM(raw)
		if !ok {
			return nil, fmt.Errorf("failed to process certificates from PEM file %q", cfg.RootCA)
		}

		out.RootCAs = roots
	}

	out.ServerName = cfg.ServerName
	if out.ServerName == "" {
		out.ServerName = serverName
	}

	if cfg.SkipVerify {
		out.InsecureSkipVerify = true
	} else if cfg.SkipVerifyServerName {
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

			if cfg.CommonName != "" {
				expectCN := cfg.CommonName
				actualCN := cs.PeerCertificates[0].Subject.CommonName
				if expectCN != actualCN {
					return fmt.Errorf("expected subject CommonName %q, got subject CommonName %q", expectCN, actualCN)
				}
			}

			return nil
		}
	} else if cfg.CommonName != "" {
		out.VerifyConnection = func(cs tls.ConnectionState) error {
			expectCN := cfg.CommonName
			actualCN := cs.PeerCertificates[0].Subject.CommonName
			if expectCN != actualCN {
				return fmt.Errorf("expected subject CommonName %q, got subject CommonName %q", expectCN, actualCN)
			}
			return nil
		}
	}

	if cfg.ClientCert != "" {
		certPath := cfg.ClientCert
		keyPath := cfg.ClientKey

		var err error
		out.Certificates = make([]tls.Certificate, 1)
		out.Certificates[0], err = tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load X.509 keypair from cert=%q key=%q: %w", certPath, keyPath, err)
		}
	}

	return out, nil
}
