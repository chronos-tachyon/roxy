package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	htmltemplate "html/template"
	"io"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"regexp"
	"time"

	zkclient "github.com/go-zookeeper/zk"
	multierror "github.com/hashicorp/go-multierror"
	log "github.com/rs/zerolog/log"
	etcdclient "go.etcd.io/etcd/client/v3"
)

var (
	reTargetKey = regexp.MustCompile(`^[A-Za-z][0-9A-Za-z]*(?:-[0-9A-Za-z]+)*$`)
)

type Impl struct {
	configPath string
	cfg        *Config
	etcd       *etcdclient.Client
	zk         *zkclient.Conn
	storage    StorageEngine
	hosts      []*regexp.Regexp
	pages      map[string]pageData
	mimeRules  []*MimeRule
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

	if impl.cfg.Storage == nil {
		return nil, ConfigLoadError{
			Path: configPath,
			Err:  fmt.Errorf("missing required section \"storage\""),
		}
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

	err = impl.loadMimeRules()
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

func (impl *Impl) loadEtcd() error {
	if impl.cfg.Etcd == nil {
		return nil
	}

	if len(impl.cfg.Etcd.Endpoints) == 0 {
		return ConfigLoadError{
			Path:    impl.configPath,
			Section: "etcd",
			Err:     fmt.Errorf("missing required field \"endpoints\""),
		}
	}

	dialTimeout := impl.cfg.Etcd.DialTimeout
	if dialTimeout == 0 {
		dialTimeout = 5 * time.Second
	}

	tlsConfig, err := CompileTLSClientConfig(impl.cfg.Etcd.TLS)
	if err != nil {
		return ConfigLoadError{
			Path:    impl.configPath,
			Section: "etcd.tls",
			Err:     err,
		}
	}

	impl.etcd, err = etcdclient.New(etcdclient.Config{
		Endpoints:            impl.cfg.Etcd.Endpoints,
		AutoSyncInterval:     1 * time.Minute,
		DialTimeout:          dialTimeout,
		DialKeepAliveTime:    impl.cfg.Etcd.KeepAliveTime,
		DialKeepAliveTimeout: impl.cfg.Etcd.KeepAliveTimeout,
		Username:             impl.cfg.Etcd.Username,
		Password:             impl.cfg.Etcd.Password,
		TLS:                  tlsConfig,
		LogConfig:            NewDummyZapConfig(),
		Context:              gRootContext,
	})
	if err != nil {
		return ConfigLoadError{
			Path:    impl.configPath,
			Section: "etcd",
			Err:     err,
		}
	}

	return nil
}

func (impl *Impl) loadZK() error {
	if impl.cfg.ZK == nil {
		return nil
	}

	if len(impl.cfg.ZK.Servers) == 0 {
		return ConfigLoadError{
			Path:    impl.configPath,
			Section: "zookeeper",
			Err:     fmt.Errorf("missing required field \"servers\""),
		}
	}

	sessTimeout := impl.cfg.ZK.SessionTimeout
	if sessTimeout == 0 {
		sessTimeout = 30 * time.Second
	}

	var err error
	impl.zk, _, err = zkclient.Connect(
		impl.cfg.ZK.Servers,
		sessTimeout,
		zkclient.WithLogger(ZKLoggerBridge{}))
	if err != nil {
		return ConfigLoadError{
			Path:    impl.configPath,
			Section: "zookeeper",
			Err:     err,
		}
	}

	if impl.cfg.ZK.Auth != nil {
		scheme := impl.cfg.ZK.Auth.Scheme
		if scheme == "" {
			return ConfigLoadError{
				Path:    impl.configPath,
				Section: "zookeeper.auth",
				Err:     fmt.Errorf("missing required field \"scheme\""),
			}
		}

		var raw []byte
		switch {
		case impl.cfg.ZK.Auth.Raw != "":
			raw, err = base64.StdEncoding.DecodeString(impl.cfg.ZK.Auth.Raw)
			if err != nil {
				return ConfigLoadError{
					Path:    impl.configPath,
					Section: "zookeeper.auth.raw",
					Err:     err,
				}
			}

		case impl.cfg.ZK.Auth.Username != "" && impl.cfg.ZK.Auth.Password != "":
			raw = []byte(impl.cfg.ZK.Auth.Username + ":" + impl.cfg.ZK.Auth.Password)

		default:
			return ConfigLoadError{
				Path:    impl.configPath,
				Section: "zookeeper.auth",
				Err:     fmt.Errorf("missing required fields \"raw\" or \"username\" + \"password\""),
			}
		}

		err = impl.zk.AddAuth(scheme, raw)
		if err != nil {
			return ConfigLoadError{
				Path:    impl.configPath,
				Section: "zookeeper.auth",
				Err:     fmt.Errorf("AddAuth %q, %s: %w", scheme, raw, err),
			}
		}
	}

	return nil
}

func (impl *Impl) loadStorageEngine() error {
	var err error
	impl.storage, err = NewStorageEngine(impl, impl.cfg.Storage)
	if err != nil {
		return ConfigLoadError{
			Path:    impl.configPath,
			Section: "storage",
			Err:     err,
		}
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

func (impl *Impl) loadPages() error {
	impl.pages = make(map[string]pageData, 64)

	if err := impl.compilePage("index", defaultIndexPageTemplate, "", ""); err != nil {
		return err
	}
	if err := impl.compilePage("redir", defaultRedirPageTemplate, "", ""); err != nil {
		return err
	}
	if err := impl.compilePage("error", defaultErrorPageTemplate, "", ""); err != nil {
		return err
	}

	if impl.cfg.Pages != nil {
		if impl.cfg.Pages.Root == "" {
			return ConfigLoadError{
				Path:    impl.configPath,
				Section: "pages",
				Err:     fmt.Errorf("missing required field \"root\""),
			}
		}

		abs, err := filepath.Abs(impl.cfg.Pages.Root)
		if err != nil {
			return ConfigLoadError{
				Path:    impl.configPath,
				Section: "pages.root",
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
	row, found := impl.cfg.Pages.Map[key]
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
			Section: fmt.Sprintf("pages.map[%q].fileName", key),
			Err:     err,
		}
	}

	return impl.compilePage(key, string(raw), contentType, contentLang, contentEnc)
}

func (impl *Impl) compilePage(key string, contents string, contentType string, contentLang string, contentEnc string) error {
	if contentType == "" && impl.cfg.Pages != nil {
		contentType = impl.cfg.Pages.DefaultContentType
	}
	if contentLang == "" && impl.cfg.Pages != nil {
		contentLang = impl.cfg.Pages.DefaultContentLang
	}
	if contentEnc == "" && impl.cfg.Pages != nil {
		contentEnc = impl.cfg.Pages.DefaultContentEnc
	}
	if contentType == "" {
		contentType = defaultContentType
	}
	if contentLang == "" {
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
			Section: fmt.Sprintf("pages.map[%q]", key),
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

func (impl *Impl) loadMimeRules() error {
	var err error
	impl.mimeRules = make([]*MimeRule, len(impl.cfg.MimeRules))
	for index, cfg := range impl.cfg.MimeRules {
		impl.mimeRules[index], err = CompileMimeRule(cfg)
		if err != nil {
			return ConfigLoadError{
				Path:    impl.configPath,
				Section: fmt.Sprintf("mimeRules[%d]", index),
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
				Section: "targets",
				Err:     fmt.Errorf("invalid backend name %q", key),
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
	var err error

	for _, handler := range impl.targets {
		if closer, ok := handler.(io.Closer); ok {
			if e := closer.Close(); e != nil {
				e = multierror.Append(err, e)
			}
		}
	}

	if e := impl.storage.Close(); e != nil {
		e = multierror.Append(err, e)
	}

	if impl.zk != nil {
		impl.zk.Close()
	}

	if impl.etcd != nil {
		if e := impl.etcd.Close(); e != nil {
			e = multierror.Append(err, e)
		}
	}

	return err
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

func (impl *Impl) HostPolicyImpl(ctx context.Context, host string) error {
	for _, rx := range impl.hosts {
		if rx.MatchString(host) {
			return nil
		}
	}
	return fmt.Errorf("unrecognized hostname %q", host)
}

func (impl *Impl) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ctx = context.WithValue(ctx, implKey{}, impl)
	r = r.WithContext(ctx)
	logger := log.Ctx(ctx)

	applicableRules := make([]*Rule, 0, len(impl.rules))
	for _, rule := range impl.rules {
		if rule.Check(r) {
			applicableRules = append(applicableRules, rule)
			if rule.IsTerminal() {
				break
			}
		}
	}

	for _, rule := range applicableRules {
		rule.ApplyFirst(w, r)
	}

	for _, rule := range applicableRules {
		rule.ApplyPre(w, r)
	}

	w.(WrappedWriter).SetRules(applicableRules, r)

	lastIndex := len(applicableRules) - 1
	target := applicableRules[lastIndex].Target
	if target != nil {
		target.ServeHTTP(w, r)
		return
	}

	r.URL.Scheme = "https"
	r.URL.Host = r.Host
	logger.Warn().Stringer("url", r.URL).Msg("no matching target")
	writeError(ctx, w, http.StatusNotFound)
}
