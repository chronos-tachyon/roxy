package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/chronos-tachyon/roxy/internal/enums"
)

type Config struct {
	Global  *GlobalConfig            `json:"global"`
	Hosts   []string                 `json:"hosts"`
	Targets map[string]*TargetConfig `json:"targets"`
	Rules   []*RuleConfig            `json:"rules"`
}

type GlobalConfig struct {
	MimeFile              string         `json:"mimeFile"`
	ACMEDirectoryURL      string         `json:"acmeDirectoryURL"`
	ACMERegistrationEmail string         `json:"acmeRegistrationEmail"`
	ACMEUserAgent         string         `json:"acmeUserAgent"`
	Etcd                  *EtcdConfig    `json:"etcd"`
	ZK                    *ZKConfig      `json:"zookeeper"`
	Storage               *StorageConfig `json:"storage"`
	Pages                 *PagesConfig   `json:"pages"`
}

type EtcdConfig struct {
	Endpoints        []string         `json:"endpoints"`
	TLS              *TLSClientConfig `json:"tls"`
	Username         string           `json:"username"`
	Password         string           `json:"password"`
	DialTimeout      time.Duration    `json:"dialTimeout"`
	KeepAliveTime    time.Duration    `json:"keepAliveTime"`
	KeepAliveTimeout time.Duration    `json:"keepAliveTimeout"`
}

type ZKConfig struct {
	Servers        []string      `json:"servers"`
	SessionTimeout time.Duration `json:"sessionTimeout"`
	Auth           *ZKAuthConfig `json:"auth"`
}

type ZKAuthConfig struct {
	Scheme   string `json:"scheme"`
	Raw      string `json:"raw"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type StorageConfig struct {
	Engine string `json:"engine"`
	Path   string `json:"path"`
}

type PagesConfig struct {
	RootDir            string                 `json:"rootDir"`
	Map                map[string]*PageConfig `json:"map"`
	DefaultContentType string                 `json:"defaultContentType"`
	DefaultContentLang string                 `json:"defaultContentLanguage"`
	DefaultContentEnc  string                 `json:"defaultContentEncoding"`
}

type PageConfig struct {
	FileName    string `json:"fileName"`
	ContentType string `json:"contentType"`
	ContentLang string `json:"contentLanguage"`
	ContentEnc  string `json:"contentEncoding"`
}

type TargetConfig struct {
	Type   enums.TargetType `json:"type"`
	Path   string           `json:"path,omitempty"`
	Target string           `json:"target,omitempty"`
	TLS    *TLSClientConfig `json:"tls,omitempty"`
}

type RuleConfig struct {
	Match     map[string]string `json:"match"`
	Mutations []*MutationConfig `json:"mutations"`
	Target    string            `json:"target"`
}

type MutationConfig struct {
	Type    enums.MutationType `json:"type"`
	Header  string             `json:"header"`
	Search  string             `json:"search"`
	Replace string             `json:"replace"`
}

type TLSClientConfig struct {
	SkipVerify        bool   `json:"skipVerify"`
	SkipVerifyDNSName bool   `json:"skipVerifyDNSName"`
	RootCA            string `json:"rootCA"`
	ExactCN           string `json:"exactCN"`
	ForceDNSName      string `json:"forceDNSName"`
	ClientCert        string `json:"clientCert"`
	ClientKey         string `json:"clientKey"`
}

type MimeFile []*MimeRuleConfig

type MimeRuleConfig struct {
	Suffixes    []string `json:"suffixes"`
	ContentType string   `json:"contentType"`
	ContentLang string   `json:"contentLanguage"`
	ContentEnc  string   `json:"contentEncoding"`
}

func CompileTLSClientConfig(cfg *TLSClientConfig) (*tls.Config, error) {
	if cfg == nil {
		return nil, nil
	}

	var roots *x509.CertPool
	var err error
	if cfg.RootCA == "" {
		roots, err = x509.SystemCertPool()
		if err != nil {
			return nil, TLSClientConfigError{
				Err: fmt.Errorf("failed to load system certificate pool: %w", err),
			}
		}
	} else {
		roots = x509.NewCertPool()

		var raw []byte
		raw, err = ioutil.ReadFile(cfg.RootCA)
		if err != nil {
			return nil, TLSClientConfigError{
				Err: fmt.Errorf("failed to read root CAs from PEM file %q: %w", cfg.RootCA, err),
			}
		}

		ok := roots.AppendCertsFromPEM(raw)
		if !ok {
			return nil, TLSClientConfigError{
				Err: fmt.Errorf("failed to process certificates from PEM file %q", cfg.RootCA),
			}
		}
	}

	out := new(tls.Config)
	if cfg.SkipVerify {
		out.InsecureSkipVerify = true
		out.VerifyConnection = func(cs tls.ConnectionState) error {
			return nil
		}
	} else if cfg.SkipVerifyDNSName || cfg.ExactCN == "" || cfg.ForceDNSName == "" {
		out.RootCAs = roots
		out.InsecureSkipVerify = true
		out.VerifyConnection = func(cs tls.ConnectionState) error {
			opts := x509.VerifyOptions{
				Roots:         roots,
				Intermediates: x509.NewCertPool(),
				DNSName:       cs.ServerName,
			}

			if cfg.ForceDNSName != "" {
				opts.DNSName = cfg.ForceDNSName
			}
			if cfg.SkipVerifyDNSName {
				opts.DNSName = ""
			}

			for _, cert := range cs.PeerCertificates[1:] {
				opts.Intermediates.AddCert(cert)
			}

			_, err := cs.PeerCertificates[0].Verify(opts)
			if err != nil {
				return err
			}

			if cfg.ExactCN != "" {
				actualCN := cs.PeerCertificates[0].Subject.CommonName
				expectCN := cfg.ExactCN
				if actualCN != expectCN {
					return fmt.Errorf("certificate subject CN %q does not match expected CN %q", actualCN, expectCN)
				}
			}

			return nil
		}
	} else {
		out.RootCAs = roots
	}

	if cfg.ClientCert != "" {
		certPath := cfg.ClientCert
		keyPath := cfg.ClientKey
		if keyPath == "" {
			keyPath = certPath
		}

		out.Certificates = make([]tls.Certificate, 1)
		out.Certificates[0], err = tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return nil, TLSClientConfigError{
				Err: fmt.Errorf("failed to load X.509 keypair from PEM files at cert=%q key=%q: %w", certPath, keyPath, err),
			}
		}
	}

	return out, nil
}
