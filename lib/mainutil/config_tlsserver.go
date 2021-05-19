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

	sawAllow := false
	for _, item := range pieces[1:] {
		switch {
		case strings.HasPrefix(item, "cert="):
			cfg.Cert = item[5:]

		case strings.HasPrefix(item, "key="):
			cfg.Key = item[4:]

		case strings.HasPrefix(item, "mTLS=") || strings.HasPrefix(item, "mtls="):
			value, err = misc.ParseBool(item[5:])
			if err != nil {
				return err
			}
			cfg.MutualTLS = value

		case strings.HasPrefix(item, "clientCA="):
			cfg.ClientCA = item[9:]

		case strings.HasPrefix(item, "ca="):
			cfg.ClientCA = item[3:]

		case strings.HasPrefix(item, "allow="):
			sawAllow = true
			err = cfg.AllowedNames.Parse(item[6:])
			if err != nil {
				return err
			}

		default:
			return fmt.Errorf("unknown option %q", item)
		}
	}

	if cfg.MutualTLS && !sawAllow {
		_ = cfg.AllowedNames.Parse(certnames.ANY)
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
		return fmt.Errorf("TLSServerConfig.Cert is empty")
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
		return nil, fmt.Errorf("failed to load X.509 keypair from cert=%q key=%q: %w", certPath, keyPath, err)
	}

	if cfg.MutualTLS {
		out.ClientAuth = tls.RequireAndVerifyClientCert

		if cfg.ClientCA == "" {
			roots, err := x509.SystemCertPool()
			if err != nil {
				return nil, fmt.Errorf("failed to load system certificate pool: %w", err)
			}

			out.ClientCAs = roots
		} else {
			roots := x509.NewCertPool()

			raw, err := ioutil.ReadFile(cfg.ClientCA)
			if err != nil {
				return nil, fmt.Errorf("failed to read file %q: %w", cfg.ClientCA, err)
			}

			ok := roots.AppendCertsFromPEM(raw)
			if !ok {
				return nil, fmt.Errorf("failed to process certificates from PEM file %q", cfg.ClientCA)
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
