package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	htmltemplate "html/template"
	"io"
	"io/fs"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-zookeeper/zk"
	multierror "github.com/hashicorp/go-multierror"
	v3 "go.etcd.io/etcd/client/v3"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

var (
	reTargetKey = regexp.MustCompile(`^[A-Za-z][0-9A-Za-z]*(?:[._+-][0-9A-Za-z]+)*$`)
)

type Impl struct {
	configPath string
	cfg        *Config
	manager    *autocert.Manager
	mimeRules  []*MimeRule
	etcd       *v3.Client
	zkconn     *zk.Conn
	storage    StorageEngine
	hosts      []*regexp.Regexp
	pages      map[string]pageData
	targets    map[string]http.Handler
	rules      []*Rule
}

type pageData struct {
	tmpl        *htmltemplate.Template
	size        int
	contentType string
	contentLang string
	contentEnc  string
}

func LoadImpl(configPath string) (*Impl, error) {
	impl := &Impl{
		configPath: configPath,
		cfg:        new(Config),
	}

	raw, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, ConfigLoadError{
			Path: configPath,
			Err:  fmt.Errorf("failed to read config file: %w", err),
		}
	}

	jsonDecoder := json.NewDecoder(bytes.NewReader(raw))
	jsonDecoder.DisallowUnknownFields()
	err = jsonDecoder.Decode(impl.cfg)
	if err != nil {
		return nil, ConfigLoadError{
			Path: configPath,
			Err:  err,
		}
	}

	err = impl.loadManager()
	if err != nil {
		return nil, err
	}

	err = impl.loadMimeRules()
	if err != nil {
		return nil, err
	}

	err = impl.loadEtcd()
	if err != nil {
		return nil, err
	}

	err = impl.loadZK()
	if err != nil {
		return nil, err
	}

	err = impl.loadStorageEngine()
	if err != nil {
		return nil, err
	}

	err = impl.loadHosts()
	if err != nil {
		return nil, err
	}

	err = impl.loadPages()
	if err != nil {
		return nil, err
	}

	err = impl.loadTargets()
	if err != nil {
		return nil, err
	}

	err = impl.loadRules()
	if err != nil {
		return nil, err
	}

	return impl, nil
}

func (impl *Impl) loadManager() error {
	acmeDirectoryURL := autocert.DefaultACMEDirectory
	acmeRegistrationEmail := ""
	acmeUserAgent := "roxy/" + Version()

	if impl.cfg.Global != nil {
		if impl.cfg.Global.ACMEDirectoryURL != "" {
			acmeDirectoryURL = impl.cfg.Global.ACMEDirectoryURL
		}
		if impl.cfg.Global.ACMERegistrationEmail != "" {
			acmeRegistrationEmail = impl.cfg.Global.ACMERegistrationEmail
		}
		if impl.cfg.Global.ACMEUserAgent != "" {
			acmeUserAgent = impl.cfg.Global.ACMEUserAgent
		}
	}

	var cache autocert.Cache = CacheWrapper{impl}
	var hostPolicy autocert.HostPolicy = impl.HostPolicyImpl
	client := &acme.Client{
		DirectoryURL: acmeDirectoryURL,
		UserAgent:    acmeUserAgent,
	}
	impl.manager = &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      cache,
		HostPolicy: hostPolicy,
		Client:     client,
		Email:      acmeRegistrationEmail,
	}
	return nil
}

func (impl *Impl) loadMimeRules() error {
	var mimeFile string
	var mimeFileJSON []byte
	var err error

	if impl.cfg.Global == nil || impl.cfg.Global.MimeFile == "" {
		mimeFile = defaultMimeFile
		mimeFileJSON, err = ioutil.ReadFile(mimeFile)
		if errors.Is(err, fs.ErrNotExist) {
			mimeFile = "<internal>"
			mimeFileJSON = []byte(defaultMimeFileJSON)
			err = nil
		}
		if err != nil {
			return ConfigLoadError{
				Path:    impl.configPath,
				Section: "global.mimeFile",
				Err:     err,
			}
		}
	} else {
		mimeFile, err := processPath(impl.cfg.Global.MimeFile)
		if err != nil {
			return ConfigLoadError{
				Path:    impl.configPath,
				Section: "global.mimeFile",
				Err:     err,
			}
		}

		mimeFileJSON, err = ioutil.ReadFile(mimeFile)
		if err != nil {
			return ConfigLoadError{
				Path:    impl.configPath,
				Section: "global.mimeFile",
				Err:     err,
			}
		}
	}

	var mimeFileData MimeFile
	d := json.NewDecoder(bytes.NewReader(mimeFileJSON))
	d.DisallowUnknownFields()
	err = d.Decode(&mimeFileData)
	if err != nil {
		return ConfigLoadError{
			Path: mimeFile,
			Err:  err,
		}
	}

	impl.mimeRules = make([]*MimeRule, len(mimeFileData))
	for index, cfg := range mimeFileData {
		impl.mimeRules[index], err = CompileMimeRule(cfg)
		if err != nil {
			return ConfigLoadError{
				Path:    mimeFile,
				Section: fmt.Sprintf("[%d]", index),
				Err:     err,
			}
		}
	}
	return nil
}

func (impl *Impl) loadEtcd() error {
	if impl.cfg.Global == nil || impl.cfg.Global.Etcd == nil {
		return nil
	}

	cfg := impl.cfg.Global.Etcd

	if len(cfg.Endpoints) == 0 {
		return ConfigLoadError{
			Path:    impl.configPath,
			Section: "global.etcd.endpoints",
			Err:     errors.New("missing required field"),
		}
	}

	dialTimeout := cfg.DialTimeout
	if dialTimeout == 0 {
		dialTimeout = 5 * time.Second
	}

	tlsConfig, err := CompileTLSClientConfig(cfg.TLS)
	if err != nil {
		return ConfigLoadError{
			Path:    impl.configPath,
			Section: "global.etcd.tls",
			Err:     err,
		}
	}

	impl.etcd, err = v3.New(v3.Config{
		Endpoints:            cfg.Endpoints,
		AutoSyncInterval:     1 * time.Minute,
		DialTimeout:          dialTimeout,
		DialKeepAliveTime:    cfg.KeepAliveTime,
		DialKeepAliveTimeout: cfg.KeepAliveTimeout,
		Username:             cfg.Username,
		Password:             cfg.Password,
		TLS:                  tlsConfig,
		LogConfig:            NewDummyZapConfig(),
		Context:              gRootContext,
	})
	if err != nil {
		return ConfigLoadError{
			Path:    impl.configPath,
			Section: "global.etcd",
			Err:     err,
		}
	}

	return nil
}

func (impl *Impl) loadZK() error {
	if impl.cfg.Global == nil || impl.cfg.Global.ZK == nil {
		return nil
	}

	cfg := impl.cfg.Global.ZK

	if len(cfg.Servers) == 0 {
		return ConfigLoadError{
			Path:    impl.configPath,
			Section: "global.zookeeper.servers",
			Err:     errors.New("missing required field"),
		}
	}

	sessTimeout := cfg.SessionTimeout
	if sessTimeout == 0 {
		sessTimeout = 30 * time.Second
	}

	var err error
	impl.zkconn, _, err = zk.Connect(
		cfg.Servers,
		sessTimeout,
		zk.WithLogger(ZKLoggerBridge{}))
	if err != nil {
		return ConfigLoadError{
			Path:    impl.configPath,
			Section: "global.zookeeper",
			Err:     err,
		}
	}

	if cfg.Auth != nil {
		scheme := cfg.Auth.Scheme
		if scheme == "" {
			return ConfigLoadError{
				Path:    impl.configPath,
				Section: "global.zookeeper.auth.scheme",
				Err:     errors.New("missing required field"),
			}
		}

		var raw []byte
		switch {
		case cfg.Auth.Raw != "":
			raw, err = base64.StdEncoding.DecodeString(cfg.Auth.Raw)
			if err != nil {
				return ConfigLoadError{
					Path:    impl.configPath,
					Section: "global.zookeeper.auth.raw",
					Err:     err,
				}
			}

		case cfg.Auth.Username != "":
			raw = []byte(cfg.Auth.Username + ":" + cfg.Auth.Password)

		default:
			return ConfigLoadError{
				Path:    impl.configPath,
				Section: "global.zookeeper.auth",
				Err:     fmt.Errorf("missing required fields \"raw\" or \"username\" + \"password\""),
			}
		}

		err = impl.zkconn.AddAuth(scheme, raw)
		if err != nil {
			return ConfigLoadError{
				Path:    impl.configPath,
				Section: "global.zookeeper.auth",
				Err:     fmt.Errorf("AddAuth %q, %s: %w", scheme, raw, err),
			}
		}
	}

	return nil
}

func (impl *Impl) loadStorageEngine() error {
	var cfg *StorageConfig
	if impl.cfg.Global == nil || impl.cfg.Global.Storage == nil {
		cfg = &StorageConfig{
			Engine: defaultStorageEngine,
			Path:   defaultStoragePath,
		}
	} else {
		cfg = impl.cfg.Global.Storage
	}

	var err error
	impl.storage, err = NewStorageEngine(impl, cfg)
	if err != nil {
		return ConfigLoadError{
			Path:    impl.configPath,
			Section: "global.storage",
			Err:     err,
		}
	}
	return nil
}

func (impl *Impl) loadPages() error {
	impl.pages = make(map[string]pageData, 64)

	if err := impl.compilePage("index", defaultIndexPageTemplate, "", "", ""); err != nil {
		return err
	}
	if err := impl.compilePage("redir", defaultRedirPageTemplate, "", "", ""); err != nil {
		return err
	}
	if err := impl.compilePage("error", defaultErrorPageTemplate, "", "", ""); err != nil {
		return err
	}

	if impl.cfg.Global != nil && impl.cfg.Global.Pages != nil {
		rootDir := impl.cfg.Global.Pages.RootDir
		if rootDir == "" {
			return ConfigLoadError{
				Path:    impl.configPath,
				Section: "global.pages.rootDir",
				Err:     errors.New("missing required field"),
			}
		}

		abs, err := processPath(rootDir)
		if err != nil {
			return ConfigLoadError{
				Path:    impl.configPath,
				Section: "global.pages.rootDir",
				Err:     err,
			}
		}

		if err := impl.loadPage("index", abs); err != nil {
			return err
		}
		if err := impl.loadPage("redir", abs); err != nil {
			return err
		}
		if err := impl.loadPage("error", abs); err != nil {
			return err
		}
		if err := impl.loadPage("4xx", abs); err != nil {
			return err
		}
		if err := impl.loadPage("5xx", abs); err != nil {
			return err
		}

		for i := 300; i < 600; i++ {
			key := fmt.Sprintf("%03d", i)
			if err := impl.loadPage(key, abs); err != nil {
				return err
			}
		}
	}

	return nil
}

func (impl *Impl) loadPage(key string, rootDir string) error {
	row, found := impl.cfg.Global.Pages.Map[key]
	if !found {
		return nil
	}

	fileName := row.FileName
	contentType := row.ContentType
	contentLang := row.ContentLang
	contentEnc := row.ContentEnc
	if fileName == "" {
		fileName = key + ".html"
	}

	raw, err := ioutil.ReadFile(filepath.Join(rootDir, fileName))
	if err != nil {
		return ConfigLoadError{
			Path:    impl.configPath,
			Section: fmt.Sprintf("global.pages.map[%q].fileName", key),
			Err:     err,
		}
	}

	return impl.compilePage(key, string(raw), contentType, contentLang, contentEnc)
}

func (impl *Impl) compilePage(key string, contents string, contentType string, contentLang string, contentEnc string) error {
	var cfg *PagesConfig
	if impl.cfg.Global != nil && impl.cfg.Global.Pages != nil {
		cfg = impl.cfg.Global.Pages
	}

	if cfg != nil {
		if contentType == "" {
			contentType = cfg.DefaultContentType
		}
		if contentLang == "" {
			contentLang = cfg.DefaultContentLang
		}
		if contentEnc == "" {
			contentEnc = cfg.DefaultContentEnc
		}
	}

	if contentType == "" {
		contentType = defaultContentType
	}
	if contentLang == "" && strings.HasPrefix(contentType, "text/") {
		contentLang = defaultContentLang
	}
	if contentEnc == "" {
		contentEnc = defaultContentEnc
	}

	t := htmltemplate.New("page")
	t = t.Funcs(htmltemplate.FuncMap{
		"runelen": runeLen,
		"uint": func(x int) uint {
			if x < 0 {
				panic(fmt.Errorf("uint: %d < 0", x))
			}
			return uint(x)
		},
		"neg": func(x uint) int {
			const max = ^uint(0) >> 1
			if x > max {
				panic(fmt.Errorf("neg: %d > %d", x, max))
			}
			return -int(x)
		},
		"add": func(a, b uint) uint {
			return a + b
		},
		"sub": func(a, b uint) uint {
			if b > a {
				panic(fmt.Errorf("sub: %d > %d", b, a))
			}
			return a - b
		},
		"pad": func(n uint) string {
			if n >= 256 {
				panic(fmt.Errorf("pad: %d >= 256", n))
			}
			buf := make([]byte, n)
			for i := uint(0); i < n; i++ {
				buf[i] = ' '
			}
			return string(buf)
		},
	})
	t, err := t.Parse(contents)
	if err != nil {
		return ConfigLoadError{
			Path:    impl.configPath,
			Section: fmt.Sprintf("global.pages.map[%q]", key),
			Err:     err,
		}
	}

	impl.pages[key] = pageData{
		tmpl:        t,
		size:        len(contents),
		contentType: contentType,
		contentLang: contentLang,
		contentEnc:  contentEnc,
	}
	return nil
}

func (impl *Impl) loadHosts() error {
	var err error
	impl.hosts = make([]*regexp.Regexp, len(impl.cfg.Hosts))
	for index, pattern := range impl.cfg.Hosts {
		impl.hosts[index], err = CompileHostGlob(pattern)
		if err != nil {
			return ConfigLoadError{
				Path:    impl.configPath,
				Section: fmt.Sprintf("hosts[%d]", index),
				Err:     err,
			}
		}
	}
	return nil
}

func (impl *Impl) loadTargets() error {
	var err error
	impl.targets = make(map[string]http.Handler, len(impl.cfg.Targets))
	for key, cfg := range impl.cfg.Targets {
		if !reTargetKey.MatchString(key) {
			return ConfigLoadError{
				Path:    impl.configPath,
				Section: fmt.Sprintf("targets[%q]", key),
				Err:     errors.New("invalid backend name"),
			}
		}

		impl.targets[key], err = CompileTarget(impl, key, cfg)
		if err != nil {
			return ConfigLoadError{
				Path:    impl.configPath,
				Section: fmt.Sprintf("targets[%q]", key),
				Err:     err,
			}
		}
	}
	return nil
}

func (impl *Impl) loadRules() error {
	var err error
	impl.rules = make([]*Rule, len(impl.cfg.Rules))
	for index, cfg := range impl.cfg.Rules {
		impl.rules[index], err = CompileRule(impl, cfg)
		if err != nil {
			return ConfigLoadError{
				Path:    impl.configPath,
				Section: fmt.Sprintf("rules[%d]", index),
				Err:     err,
			}
		}
	}
	return nil
}

func (impl *Impl) Close() error {
	var errs multierror.Error

	for _, handler := range impl.targets {
		if closer, ok := handler.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				errs.Errors = append(errs.Errors, err)
			}
		}
	}

	if err := impl.storage.Close(); err != nil {
		errs.Errors = append(errs.Errors, err)
	}

	if impl.zkconn != nil {
		impl.zkconn.Close()
	}

	if impl.etcd != nil {
		if err := impl.etcd.Close(); err != nil {
			errs.Errors = append(errs.Errors, err)
		}
	}

	return errs.ErrorOrNil()
}

func (impl *Impl) ACMEManager() *autocert.Manager {
	return impl.manager
}

func (impl *Impl) HostPolicyImpl(ctx context.Context, host string) error {
	for _, rx := range impl.hosts {
		if rx.MatchString(host) {
			return nil
		}
	}
	return fmt.Errorf("unrecognized hostname %q", host)
}

func (impl *Impl) StorageGet(ctx context.Context, key string) ([]byte, error) {
	return impl.storage.Get(ctx, key)
}

func (impl *Impl) StoragePut(ctx context.Context, key string, data []byte) error {
	return impl.storage.Put(ctx, key, data)
}

func (impl *Impl) StorageDelete(ctx context.Context, key string) error {
	return impl.storage.Delete(ctx, key)
}

type CacheWrapper struct {
	impl *Impl
}

func (wrap CacheWrapper) Get(ctx context.Context, key string) ([]byte, error) {
	return wrap.impl.StorageGet(ctx, key)
}

func (wrap CacheWrapper) Put(ctx context.Context, key string, data []byte) error {
	return wrap.impl.StoragePut(ctx, key, data)
}

func (wrap CacheWrapper) Delete(ctx context.Context, key string) error {
	return wrap.impl.StorageDelete(ctx, key)
}

var _ autocert.Cache = CacheWrapper{}
