package main

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

type Rule struct {
	Match         map[string]*regexp.Regexp
	Mutations     []Mutation
	TargetKey     string
	TargetConfig  *TargetConfig
	TargetHandler http.Handler
}

func (rule *Rule) Check(r *http.Request) bool {
	for name, rx := range rule.Match {
		var values []string
		switch {
		case strings.EqualFold(name, "host"):
			values = []string{r.Host}
		case strings.EqualFold(name, "method"):
			values = []string{r.Method}
		case strings.EqualFold(name, "path"):
			values = []string{r.URL.Path}
		default:
			values = r.Header.Values(name)
		}
		any := false
		for _, value := range values {
			if rx.MatchString(value) {
				any = true
				break
			}
		}
		if !any {
			return false
		}
	}
	return true
}

func (rule *Rule) IsTerminal() bool {
	return rule.TargetKey != ""
}

func (rule *Rule) ApplyFirst(w http.ResponseWriter, r *http.Request) {
	for _, mutation := range rule.Mutations {
		mutation.ApplyFirst(w, r)
	}
}

func (rule *Rule) ApplyPre(w http.ResponseWriter, r *http.Request) {
	for _, mutation := range rule.Mutations {
		mutation.ApplyPre(w, r)
	}
}

func (rule *Rule) ApplyPost(w http.ResponseWriter, r *http.Request) {
	for _, mutation := range rule.Mutations {
		mutation.ApplyPost(w, r)
	}
}

func CompileRule(impl *Impl, cfg *RuleConfig) (*Rule, error) {
	var err error

	out := new(Rule)

	if len(cfg.Match) != 0 {
		out.Match = make(map[string]*regexp.Regexp, len(cfg.Match))
		for name, pattern := range cfg.Match {
			out.Match[name], err = regexp.Compile(`^` + pattern + `$`)
			if err != nil {
				return nil, fmt.Errorf("match[%q]: failed to compile regex /^%s$/: %w", name, pattern, err)
			}
		}
	}

	out.Mutations = make([]Mutation, len(cfg.Mutations))
	for i, mutcfg := range cfg.Mutations {
		out.Mutations[i], err = CompileMutation(mutcfg)
		if err != nil {
			return nil, fmt.Errorf("mutations[%d]: %w", i, err)
		}
	}

	out.TargetKey = cfg.Target
	switch {
	case out.TargetKey == "":
		// pass

	case strings.HasPrefix(out.TargetKey, "ERROR:"):
		out.TargetHandler, err = CompileErrorHandler(impl, out.TargetKey)
		if err != nil {
			return nil, err
		}

	case strings.HasPrefix(out.TargetKey, "REDIR:"):
		out.TargetHandler, err = CompileRedirHandler(impl, out.TargetKey)
		if err != nil {
			return nil, err
		}

	default:
		out.TargetConfig = impl.cfg.Targets[out.TargetKey]
		out.TargetHandler = impl.targets[out.TargetKey]
		if out.TargetHandler == nil {
			return nil, fmt.Errorf("unknown target %q", out.TargetKey)
		}
	}

	return out, nil
}
