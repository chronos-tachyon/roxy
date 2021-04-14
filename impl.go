package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	htmltemplate "html/template"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"regexp"

	log "github.com/rs/zerolog/log"
	autocert "golang.org/x/crypto/acme/autocert"
)

var (
	reTargetKey = regexp.MustCompile(`^[A-Za-z][0-9A-Za-z]*(?:-[0-9A-Za-z]+)*$`)
)

type Impl struct {
	cfg           *Config
	storage       autocert.Cache
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

	impl.storage, err = NewStorageEngine(impl.cfg.Storage)
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
	var err error
	if x, ok := impl.storage.(interface{ Close() error }); ok {
		err = x.Close()
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
