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

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

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
		optName, optValue, optComplete, err := splitOption(item)
		if err != nil {
			return err
		}

		optErr := OptionError{
			Name:     optName,
			Value:    optValue,
			Complete: optComplete,
		}

		switch optName {
		case optionVerify:
			value, err = misc.ParseBool(optValue)
			if err != nil {
				optErr.Err = err
				return optErr
			}
			cfg.SkipVerify = !value

		case optionVerifyServerName:
			value, err = misc.ParseBool(optValue)
			if err != nil {
				optErr.Err = err
				return optErr
			}
			cfg.SkipVerifyServerName = !value

		case optionCA:
			fallthrough
		case optionServerCA:
			cfg.RootCA = optValue

		case optionSN:
			fallthrough
		case optionServerName:
			cfg.ServerName = optValue

		case optionCN:
			fallthrough
		case optionCommonName:
			cfg.CommonName = optValue

		case optionCert:
			fallthrough
		case optionClientCert:
			cfg.ClientCert = optValue

		case optionKey:
			fallthrough
		case optionClientKey:
			cfg.ClientKey = optValue

		default:
			optErr.Err = UnknownOptionError{}
			return optErr
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

// MakeTLS constructs the configured *tls.Config, or returns nil if TLS is
// disabled.
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
			return nil, CertPoolError{Err: err}
		}

		out.RootCAs = roots
	} else {
		roots := x509.NewCertPool()

		raw, err := ioutil.ReadFile(cfg.RootCA)
		if err != nil {
			return nil, CertPoolError{Path: cfg.RootCA, Err: err}
		}

		ok := roots.AppendCertsFromPEM(raw)
		if !ok {
			return nil, CertPoolError{Path: cfg.RootCA, Err: ErrOperationFailed}
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
			return nil, CertKeyError{CertPath: certPath, KeyPath: keyPath, Err: err}
		}
	}

	return out, nil
}

// MakeDialOption returns a grpc.DialOption that uses the result of MakeTLS as
// TLS transport credentials, or returns grpc.WithInsecure() if TLS is
// disabled.
func (cfg TLSClientConfig) MakeDialOption(serverName string) (grpc.DialOption, error) {
	tc, err := cfg.MakeTLS(serverName)
	if err != nil {
		return grpc.EmptyDialOption{}, err
	}

	if tc == nil {
		return grpc.WithInsecure(), nil
	}

	return grpc.WithTransportCredentials(credentials.NewTLS(tc)), nil
}
