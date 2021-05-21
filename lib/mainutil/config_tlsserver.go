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
	"github.com/chronos-tachyon/roxy/lib/certnames"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// TLSServerConfig represents the configuration for a server-oriented
// *tls.Config object.
type TLSServerConfig struct {
	Enabled      bool
	Cert         string
	Key          string
	MutualTLS    bool
	ClientCA     string
	AllowedNames certnames.CertNames
}

// TLSServerConfigJSON represents the JSON doppelgänger of an TLSServerConfig.
type TLSServerConfigJSON struct {
	Cert         string              `json:"cert,omitempty"`
	Key          string              `json:"key,omitempty"`
	MutualTLS    bool                `json:"mTLS,omitempty"`
	ClientCA     string              `json:"clientCA,omitempty"`
	AllowedNames certnames.CertNames `json:"allowedNames,omitempty"`
}

// AppendTo appends the string representation to the given Builder.
func (cfg TLSServerConfig) AppendTo(out *strings.Builder) {
	if !cfg.Enabled {
		out.WriteString("no")
		return
	}
	out.WriteString("yes")
	out.WriteString(",cert=")
	out.WriteString(cfg.Cert)
	if cfg.Key != "" && cfg.Key != cfg.Cert {
		out.WriteString(",key=")
		out.WriteString(cfg.Key)
	}
	if cfg.MutualTLS {
		out.WriteString(",mtls=yes")
		if cfg.ClientCA != "" {
			out.WriteString(",ca=")
			out.WriteString(cfg.ClientCA)
		}
		out.WriteString(",allow=")
		cfg.AllowedNames.AppendTo(out)
	}
}

// String returns the string representation.
func (cfg TLSServerConfig) String() string {
	if !cfg.Enabled {
		return "no"
	}

	var buf strings.Builder
	buf.Grow(64)
	cfg.AppendTo(&buf)
	return buf.String()
}

// MarshalJSON fulfills json.Marshaler.
func (cfg TLSServerConfig) MarshalJSON() ([]byte, error) {
	if !cfg.Enabled {
		return constants.NullBytes, nil
	}
	return json.Marshal(cfg.ToJSON())
}

// ToJSON converts the object to its JSON doppelgänger.
func (cfg TLSServerConfig) ToJSON() *TLSServerConfigJSON {
	if !cfg.Enabled {
		return nil
	}
	return &TLSServerConfigJSON{
		Cert:         cfg.Cert,
		Key:          cfg.Key,
		MutualTLS:    cfg.MutualTLS,
		ClientCA:     cfg.ClientCA,
		AllowedNames: cfg.AllowedNames,
	}
}

// Parse parses the string representation.
func (cfg *TLSServerConfig) Parse(str string) error {
	if cfg == nil {
		panic(errors.New("*TLSServerConfig is nil"))
	}

	wantZero := true
	defer func() {
		if wantZero {
			*cfg = TLSServerConfig{}
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
		case optionCert:
			fallthrough
		case optionServerCert:
			cfg.Cert = optValue

		case optionKey:
			fallthrough
		case optionServerKey:
			cfg.Key = optValue

		case optionMTLS:
			value, err = misc.ParseBool(optValue)
			if err != nil {
				optErr.Err = err
				return optErr
			}
			cfg.MutualTLS = value

		case optionCA:
			fallthrough
		case optionClientCA:
			cfg.ClientCA = optValue

		case optionAllow:
			err = cfg.AllowedNames.Parse(optValue)
			if err != nil {
				optErr.Err = err
				return optErr
			}

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
func (cfg *TLSServerConfig) UnmarshalJSON(raw []byte) error {
	if cfg == nil {
		panic(errors.New("*TLSServerConfig is nil"))
	}

	wantZero := true
	defer func() {
		if wantZero {
			*cfg = TLSServerConfig{}
		}
	}()

	if bytes.Equal(raw, constants.NullBytes) {
		return nil
	}

	var alt *TLSServerConfigJSON
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
func (cfg *TLSServerConfig) FromJSON(alt *TLSServerConfigJSON) error {
	if cfg == nil {
		panic(errors.New("*TLSServerConfig is nil"))
	}

	if alt == nil {
		*cfg = TLSServerConfig{}
		return nil
	}

	*cfg = TLSServerConfig{
		Enabled:      true,
		Cert:         alt.Cert,
		Key:          alt.Key,
		MutualTLS:    alt.MutualTLS,
		ClientCA:     alt.ClientCA,
		AllowedNames: alt.AllowedNames,
	}

	return nil
}

// PostProcess performs data integrity checks and input post-processing.
func (cfg *TLSServerConfig) PostProcess() error {
	if cfg == nil {
		panic(errors.New("*TLSServerConfig is nil"))
	}

	if !cfg.Enabled {
		*cfg = TLSServerConfig{}
		return nil
	}

	if cfg.Cert == "" {
		return roxyutil.StructFieldError{
			Field: "TLSServerConfig.Cert",
			Value: cfg.Cert,
			Err:   roxyutil.ErrExpectNonEmpty,
		}
	}

	expanded, err := roxyutil.ExpandPath(cfg.Cert)
	if err != nil {
		return err
	}
	cfg.Cert = expanded

	if cfg.Key != "" {
		expanded, err = roxyutil.ExpandPath(cfg.Key)
		if err != nil {
			return err
		}
		cfg.Key = expanded
	}

	if cfg.Key == "" {
		cfg.Key = cfg.Cert
	}

	if !cfg.MutualTLS {
		cfg.ClientCA = ""
		_ = cfg.AllowedNames.Parse(certnames.ANY)
	}

	if cfg.ClientCA != "" {
		expanded, err = roxyutil.ExpandPath(cfg.ClientCA)
		if err != nil {
			return err
		}
		cfg.ClientCA = expanded
	}

	return nil
}

// MakeTLS constructs the configured *tls.Config.
func (cfg TLSServerConfig) MakeTLS() (*tls.Config, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	out := new(tls.Config)

	certPath := cfg.Cert
	keyPath := cfg.Key

	var err error
	out.Certificates = make([]tls.Certificate, 1)
	out.Certificates[0], err = tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, CertKeyError{CertPath: certPath, KeyPath: keyPath, Err: err}
	}

	if cfg.MutualTLS {
		out.ClientAuth = tls.RequireAndVerifyClientCert

		if cfg.ClientCA == "" {
			roots, err := x509.SystemCertPool()
			if err != nil {
				return nil, CertPoolError{Err: err}
			}

			out.ClientCAs = roots
		} else {
			roots := x509.NewCertPool()

			raw, err := ioutil.ReadFile(cfg.ClientCA)
			if err != nil {
				return nil, CertPoolError{Path: cfg.ClientCA, Err: err}
			}

			ok := roots.AppendCertsFromPEM(raw)
			if !ok {
				return nil, CertPoolError{Path: cfg.ClientCA, Err: ErrOperationFailed}
			}

			out.ClientCAs = roots
		}

		out.VerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			cert := verifiedChains[0][0]
			if cfg.AllowedNames.Check(cert) {
				return nil
			}
			return fmt.Errorf("expected %s, got Subject DN %q", cfg.AllowedNames.String(), cert.Subject.String())
		}
	}

	out.NextProtos = []string{"h2", "http/1.1"}
	return out, nil
}

// MakeGRPCServerOptions constructs the configured gRPC ServerOptions.
func (cfg TLSServerConfig) MakeGRPCServerOptions(opts ...grpc.ServerOption) ([]grpc.ServerOption, error) {
	if !cfg.Enabled {
		return opts, nil
	}

	tlsConfig, err := cfg.MakeTLS()
	if err != nil {
		return nil, err
	}

	tlsConfig.NextProtos = []string{"h2"}

	out := make([]grpc.ServerOption, 1+len(opts))
	out[0] = grpc.Creds(credentials.NewTLS(tlsConfig))
	copy(out[1:], opts)
	return out, nil
}
