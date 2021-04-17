package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	htmltemplate "html/template"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"regexp"
	"time"

	zkclient "github.com/go-zookeeper/zk"
	log "github.com/rs/zerolog/log"
	etcdclient "go.etcd.io/etcd/client/v3"
)

var (
	reTargetKey = regexp.MustCompile(`^[A-Za-z][0-9A-Za-z]*(?:-[0-9A-Za-z]+)*$`)
)

type Impl struct {
	cfg           *Config
	etcd          *etcdclient.Client
	zk            *zkclient.Conn
	storage       StorageEngine
	hosts         []*regexp.Regexp
	errorPageRoot string
	indexPageTmpl *htmltemplate.Template
	mimeRules     []*MimeRule
	targets       map[string]http.Handler
	rules         []*Rule
}

func LoadImpl(configPath string) (*Impl, error) {
	impl := new(Impl)

	raw, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, ConfigLoadError{
			Path: configPath,
			Err:  err,
		}
	}

	impl.cfg = new(Config)
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

	if impl.cfg.Etcd != nil {
		if len(impl.cfg.Etcd.Endpoints) == 0 {
			return nil, ConfigLoadError{
				Path:    configPath,
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
			return nil, ConfigLoadError{
				Path:    configPath,
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
			return nil, ConfigLoadError{
				Path:    configPath,
				Section: "etcd",
				Err:     err,
			}
		}
	}

	if impl.cfg.ZK != nil {
		if len(impl.cfg.ZK.Servers) == 0 {
			return nil, ConfigLoadError{
				Path:    configPath,
				Section: "zookeeper",
				Err:     fmt.Errorf("missing required field \"servers\""),
			}
		}

		sessTimeout := impl.cfg.ZK.SessionTimeout
		if sessTimeout == 0 {
			sessTimeout = 30 * time.Second
		}

		impl.zk, _, err = zkclient.Connect(
			impl.cfg.ZK.Servers,
			sessTimeout,
			zkclient.WithLogger(ZKLoggerBridge{}))
		if err != nil {
			return nil, ConfigLoadError{
				Path:    configPath,
				Section: "zookeeper",
				Err:     err,
			}
		}

		if impl.cfg.ZK.Auth != nil {
			scheme := impl.cfg.ZK.Auth.Scheme
			if scheme == "" {
				return nil, ConfigLoadError{
					Path:    configPath,
					Section: "zookeeper.auth",
					Err:     fmt.Errorf("missing required field \"scheme\""),
				}
			}

			var raw []byte
			switch {
			case impl.cfg.ZK.Auth.Raw != "":
				raw, err = base64.StdEncoding.DecodeString(impl.cfg.ZK.Auth.Raw)
				if err != nil {
					return nil, ConfigLoadError{
						Path:    configPath,
						Section: "zookeeper.auth.raw",
						Err:     err,
					}
				}

			case impl.cfg.ZK.Auth.Username != "" && impl.cfg.ZK.Auth.Password != "":
				raw = []byte(impl.cfg.ZK.Auth.Username + ":" + impl.cfg.ZK.Auth.Password)

			default:
				return nil, ConfigLoadError{
					Path:    configPath,
					Section: "zookeeper.auth",
					Err:     fmt.Errorf("missing required fields \"raw\" or \"username\" + \"password\""),
				}
			}

			err = impl.zk.AddAuth(scheme, raw)
			if err != nil {
				return nil, ConfigLoadError{
					Path:    configPath,
					Section: "zookeeper.auth",
					Err:     fmt.Errorf("AddAuth %q, %s: %w", scheme, raw, err),
				}
			}
		}
	}

	impl.storage, err = NewStorageEngine(impl, impl.cfg.Storage)
	if err != nil {
		return nil, ConfigLoadError{
			Path:    configPath,
			Section: "storage",
			Err:     err,
		}
	}

	impl.hosts = make([]*regexp.Regexp, len(impl.cfg.Hosts))
	for i, pattern := range impl.cfg.Hosts {
		impl.hosts[i], err = CompileHostGlob(pattern)
		if err != nil {
			return nil, ConfigLoadError{
				Path:    configPath,
				Section: fmt.Sprintf("hosts[%d]", i),
				Err:     err,
			}
		}
	}

	if impl.cfg.ErrorPages != nil && impl.cfg.ErrorPages.Root != "" {
		impl.errorPageRoot, err = filepath.Abs(impl.cfg.ErrorPages.Root)
		if err != nil {
			return nil, ConfigLoadError{
				Path:    configPath,
				Section: "errorPages.root",
				Err:     err,
			}
		}
	}

	indexPageTemplate := defaultIndexPageTemplate
	if impl.cfg.IndexPages != nil && impl.cfg.IndexPages.Path != "" {
		contents, err := ioutil.ReadFile(impl.cfg.IndexPages.Path)
		if err != nil {
			return nil, ConfigLoadError{
				Path:    configPath,
				Section: "indexPages.path",
				Err:     err,
			}
		}

		indexPageTemplate = string(contents)
	}

	impl.indexPageTmpl = htmltemplate.New("index").Funcs(htmltemplate.FuncMap{
		"runelen": runeLen,
		"uint": func(x int) uint {
			if x < 0 {
				panic(fmt.Errorf("uint: %d < 0", x))
			}
			return uint(x)
		},
		"neg": func(x uint) int {
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
	impl.indexPageTmpl, err = impl.indexPageTmpl.Parse(indexPageTemplate)
	if err != nil {
		return nil, ConfigLoadError{
			Path:    configPath,
			Section: "indexPages.path",
			Err:     err,
		}
	}

	impl.mimeRules = make([]*MimeRule, len(impl.cfg.MimeRules))
	for i, cfg := range impl.cfg.MimeRules {
		impl.mimeRules[i], err = CompileMimeRule(cfg)
		if err != nil {
			return nil, ConfigLoadError{
				Path:    configPath,
				Section: fmt.Sprintf("mimeRules[%d]", i),
				Err:     err,
			}
		}
	}

	impl.targets = make(map[string]http.Handler, len(impl.cfg.Targets))
	for key, cfg := range impl.cfg.Targets {
		if !reTargetKey.MatchString(key) {
			return nil, ConfigLoadError{
				Path:    configPath,
				Section: "targets",
				Err:     fmt.Errorf("invalid backend name %q", key),
			}
		}

		impl.targets[key], err = CompileTarget(impl, key, cfg)
		if err != nil {
			return nil, ConfigLoadError{
				Path:    configPath,
				Section: fmt.Sprintf("targets[%q]", key),
				Err:     err,
			}
		}
	}

	impl.rules = make([]*Rule, len(impl.cfg.Rules))
	for i, cfg := range impl.cfg.Rules {
		impl.rules[i], err = CompileRule(impl, cfg)
		if err != nil {
			return nil, ConfigLoadError{
				Path:    configPath,
				Section: fmt.Sprintf("rules[%d]", i),
				Err:     err,
			}
		}
	}

	return impl, nil
}

func (impl *Impl) Close() error {
	err0 := impl.storage.Close()

	if impl.zk != nil {
		impl.zk.Close()
	}

	var err1 error
	if impl.etcd != nil {
		err1 = impl.etcd.Close()
	}

	switch {
	case err0 != nil:
		return err0
	case err1 != nil:
		return err1
	default:
		return nil
	}
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
