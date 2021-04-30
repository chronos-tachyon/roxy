package main

import (
	"bytes"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"text/template"

	"github.com/rs/zerolog/log"

	"github.com/chronos-tachyon/roxy/internal/enums"
)

var (
	reEmptyString = regexp.MustCompile(`^$`)
)

// type Mutation {{{

type Mutation interface {
	ApplyFirst(w http.ResponseWriter, r *http.Request)
	ApplyPre(w http.ResponseWriter, r *http.Request)
	ApplyPost(w http.ResponseWriter, r *http.Request)
}

// type RequestHostMutation {{{

type RequestHostMutation struct {
	Search  *regexp.Regexp
	Replace *template.Template
}

func (mut *RequestHostMutation) ApplyFirst(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	oldValue := r.Host

	captures := mut.Search.FindStringSubmatch(oldValue)
	if captures == nil {
		return
	}

	var buf bytes.Buffer
	err := mut.Replace.Execute(&buf, captures)
	if err != nil {
		logger.Warn().
			Str("type", "req-host").
			Str("value", oldValue).
			Stringer("pattern", mut.Search).
			Strs("captures", captures).
			Err(err).
			Msg("failed to execute template")
		return
	}
	newValue := buf.String()

	logger.Debug().
		Str("type", "req-host").
		Str("oldValue", oldValue).
		Str("newValue", newValue).
		Msg("rewrite")
	r.Host = newValue
}

func (mut *RequestHostMutation) ApplyPre(w http.ResponseWriter, r *http.Request) {
	// pass
}

func (mut *RequestHostMutation) ApplyPost(w http.ResponseWriter, r *http.Request) {
	// pass
}

var _ Mutation = (*RequestHostMutation)(nil)

// }}}

// type RequestPathMutation {{{

type RequestPathMutation struct {
	Search  *regexp.Regexp
	Replace *template.Template
}

func (mut *RequestPathMutation) ApplyFirst(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	oldValue := r.URL.Path

	captures := mut.Search.FindStringSubmatch(oldValue)
	if captures == nil {
		return
	}

	var buf bytes.Buffer
	err := mut.Replace.Execute(&buf, captures)
	if err != nil {
		logger.Warn().
			Str("type", "req-path").
			Str("value", oldValue).
			Stringer("pattern", mut.Search).
			Strs("captures", captures).
			Err(err).
			Msg("failed to execute template")
		return
	}
	newValue := buf.String()

	logger.Debug().
		Str("type", "req-path").
		Str("oldValue", oldValue).
		Str("newValue", newValue).
		Msg("rewrite")
	r.URL.Path = newValue
}

func (mut *RequestPathMutation) ApplyPre(w http.ResponseWriter, r *http.Request) {
	// pass
}

func (mut *RequestPathMutation) ApplyPost(w http.ResponseWriter, r *http.Request) {
	// pass
}

var _ Mutation = (*RequestPathMutation)(nil)

// }}}

// type RequestQueryMutation {{{

type RequestQueryMutation struct {
	Search  *regexp.Regexp
	Replace *template.Template
}

func (mut *RequestQueryMutation) ApplyFirst(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	oldValue := "&" + r.URL.RawQuery + "&"

	captures := mut.Search.FindStringSubmatch(oldValue)
	if captures == nil {
		return
	}

	var buf bytes.Buffer
	err := mut.Replace.Execute(&buf, captures)
	if err != nil {
		logger.Warn().
			Str("type", "req-query").
			Str("value", oldValue).
			Stringer("pattern", mut.Search).
			Strs("captures", captures).
			Err(err).
			Msg("failed to execute template")
		return
	}
	newValue := buf.String()

	logger.Debug().
		Str("type", "req-query").
		Str("oldValue", oldValue).
		Str("newValue", newValue).
		Msg("rewrite")
	r.URL.RawQuery = strings.Trim(newValue, "&")
}

func (mut *RequestQueryMutation) ApplyPre(w http.ResponseWriter, r *http.Request) {
	// pass
}

func (mut *RequestQueryMutation) ApplyPost(w http.ResponseWriter, r *http.Request) {
	// pass
}

var _ Mutation = (*RequestQueryMutation)(nil)

// }}}

// type RequestHeaderMutation {{{

type RequestHeaderMutation struct {
	Header  string
	Search  *regexp.Regexp
	Replace *template.Template
}

func (mut *RequestHeaderMutation) ApplyFirst(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	oldValue := hdrGet(r.Header, mut.Header)

	captures := mut.Search.FindStringSubmatch(oldValue)
	if captures == nil {
		return
	}

	var buf bytes.Buffer
	err := mut.Replace.Execute(&buf, captures)
	if err != nil {
		logger.Warn().
			Str("type", "req-header").
			Str("header", mut.Header).
			Str("value", oldValue).
			Stringer("pattern", mut.Search).
			Strs("captures", captures).
			Err(err).
			Msg("failed to execute template")
		return
	}
	newValue := buf.String()

	logger.Debug().
		Str("type", "req-header").
		Str("header", mut.Header).
		Str("oldValue", oldValue).
		Str("newValue", newValue).
		Msg("rewrite")
	hdrSet(r.Header, mut.Header, newValue)
}

func (mut *RequestHeaderMutation) ApplyPre(w http.ResponseWriter, r *http.Request) {
	// pass
}

func (mut *RequestHeaderMutation) ApplyPost(w http.ResponseWriter, r *http.Request) {
	// pass
}

var _ Mutation = (*RequestHeaderMutation)(nil)

// }}}

// type ResponseHeaderPreMutation {{{

type ResponseHeaderPreMutation struct {
	Header  string
	Search  *regexp.Regexp
	Replace *template.Template
}

func (mut *ResponseHeaderPreMutation) ApplyFirst(w http.ResponseWriter, r *http.Request) {
	// pass
}

func (mut *ResponseHeaderPreMutation) ApplyPre(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())
	headers := w.Header()

	oldValue := hdrGet(headers, mut.Header)

	captures := mut.Search.FindStringSubmatch(oldValue)
	if captures == nil {
		return
	}

	var buf bytes.Buffer
	err := mut.Replace.Execute(&buf, captures)
	if err != nil {
		logger.Warn().
			Str("type", "resp-header").
			Str("header", mut.Header).
			Str("value", oldValue).
			Stringer("pattern", mut.Search).
			Strs("captures", captures).
			Err(err).
			Msg("failed to execute template")
		return
	}
	newValue := buf.String()

	logger.Debug().
		Str("type", "resp-header").
		Str("header", mut.Header).
		Str("oldValue", oldValue).
		Str("newValue", newValue).
		Msg("rewrite")
	hdrSet(headers, mut.Header, newValue)
}

func (mut *ResponseHeaderPreMutation) ApplyPost(w http.ResponseWriter, r *http.Request) {
	// pass
}

var _ Mutation = (*ResponseHeaderPreMutation)(nil)

// }}}

// type ResponseHeaderPostMutation {{{

type ResponseHeaderPostMutation struct {
	Header  string
	Search  *regexp.Regexp
	Replace *template.Template
}

func (mut *ResponseHeaderPostMutation) ApplyFirst(w http.ResponseWriter, r *http.Request) {
	// pass
}

func (mut *ResponseHeaderPostMutation) ApplyPre(w http.ResponseWriter, r *http.Request) {
	// pass
}

func (mut *ResponseHeaderPostMutation) ApplyPost(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())
	headers := w.Header()

	oldValue := hdrGet(headers, mut.Header)

	captures := mut.Search.FindStringSubmatch(oldValue)
	if captures == nil {
		return
	}

	var buf bytes.Buffer
	err := mut.Replace.Execute(&buf, captures)
	if err != nil {
		logger.Warn().
			Str("type", "resp-header-post").
			Str("header", mut.Header).
			Str("value", oldValue).
			Stringer("pattern", mut.Search).
			Strs("captures", captures).
			Err(err).
			Msg("failed to execute template")
		return
	}
	newValue := buf.String()

	logger.Debug().
		Str("type", "resp-header-post").
		Str("header", mut.Header).
		Str("oldValue", oldValue).
		Str("newValue", newValue).
		Msg("rewrite")
	hdrSet(headers, mut.Header, newValue)
}

var _ Mutation = (*ResponseHeaderPostMutation)(nil)

// }}}

// }}}

func CompileMutation(cfg *MutationConfig) (Mutation, error) {
	var (
		searchRx    *regexp.Regexp
		replaceTmpl *template.Template
		err         error
	)

	if cfg.Search == "" {
		searchRx = reEmptyString
	} else {
		searchRx, err = regexp.Compile(`^` + cfg.Search + `$`)
		if err != nil {
			return nil, fmt.Errorf("\"search\": failed to compile regexp /^%s$/: %w", cfg.Search, err)
		}
	}

	massaged := massageTemplate(cfg.Replace)
	replaceTmpl = template.New("rewrite")
	replaceTmpl, err = replaceTmpl.Parse(massaged)
	if err != nil {
		return nil, fmt.Errorf("\"replace\": failed to compile text/template %q: %w", massaged, err)
	}

	switch cfg.Type {
	case enums.UndefinedMutationType:
		return nil, fmt.Errorf("missing required field \"type\"")

	case enums.RequestHostMutationType:
		if cfg.Header != "" {
			return nil, fmt.Errorf("unexpected field \"header\": %q", cfg.Header)
		}

		return &RequestHostMutation{
			Search:  searchRx,
			Replace: replaceTmpl,
		}, nil

	case enums.RequestPathMutationType:
		if cfg.Header != "" {
			return nil, fmt.Errorf("unexpected field \"header\": %q", cfg.Header)
		}

		return &RequestPathMutation{
			Search:  searchRx,
			Replace: replaceTmpl,
		}, nil

	case enums.RequestQueryMutationType:
		if cfg.Header != "" {
			return nil, fmt.Errorf("unexpected field \"header\": %q", cfg.Header)
		}

		return &RequestQueryMutation{
			Search:  searchRx,
			Replace: replaceTmpl,
		}, nil

	case enums.RequestHeaderMutationType:
		if cfg.Header == "" {
			return nil, fmt.Errorf("missing required field \"header\"")
		}

		return &RequestHeaderMutation{
			Header:  cfg.Header,
			Search:  searchRx,
			Replace: replaceTmpl,
		}, nil

	case enums.ResponseHeaderPreMutationType:
		if cfg.Header == "" {
			return nil, fmt.Errorf("missing required field \"header\"")
		}

		return &ResponseHeaderPreMutation{
			Header:  cfg.Header,
			Search:  searchRx,
			Replace: replaceTmpl,
		}, nil

	case enums.ResponseHeaderPostMutationType:
		if cfg.Header == "" {
			return nil, fmt.Errorf("missing required field \"header\"")
		}

		return &ResponseHeaderPostMutation{
			Header:  cfg.Header,
			Search:  searchRx,
			Replace: replaceTmpl,
		}, nil

	default:
		return nil, fmt.Errorf("%#v not implemented", cfg.Type)
	}

	return nil, nil
}

func hdrGet(hdrs http.Header, name string) string {
	values := hdrs.Values(name)
	switch len(values) {
	case 0:
		return ""
	case 1:
		return values[0]
	default:
		return strings.Join(values, ", ")
	}
}

func hdrSet(hdrs http.Header, name string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		hdrs.Del(name)
	} else {
		hdrs.Set(name, value)
	}
}

func massageTemplate(in string) string {
	const (
		stateReady = iota
		stateOneOpen
		stateTwoOpen
		stateTwoOpenOneClose
		stateBackslash
	)

	var state uint

	var buf strings.Builder
	buf.Grow(len(in))

	for _, ch := range in {
		switch state {
		case stateReady:
			switch ch {
			case '{':
				buf.WriteByte('{')
				state = stateOneOpen
			case '\\':
				state = stateBackslash
			default:
				buf.WriteRune(ch)
			}

		case stateOneOpen:
			switch ch {
			case '{':
				buf.WriteByte('{')
				state = stateTwoOpen
			case '\\':
				state = stateBackslash
			default:
				buf.WriteRune(ch)
				state = stateReady
			}

		case stateTwoOpen:
			switch ch {
			case '}':
				buf.WriteByte('}')
				state = stateTwoOpenOneClose
			default:
				buf.WriteRune(ch)
			}

		case stateTwoOpenOneClose:
			switch ch {
			case '}':
				buf.WriteByte('}')
				state = stateReady
			default:
				buf.WriteRune(ch)
				state = stateTwoOpen
			}

		case stateBackslash:
			switch ch {
			case '0':
				buf.WriteString("{{index . 0}}")
			case '1':
				buf.WriteString("{{index . 1}}")
			case '2':
				buf.WriteString("{{index . 2}}")
			case '3':
				buf.WriteString("{{index . 3}}")
			case '4':
				buf.WriteString("{{index . 4}}")
			case '5':
				buf.WriteString("{{index . 5}}")
			case '6':
				buf.WriteString("{{index . 6}}")
			case '7':
				buf.WriteString("{{index . 7}}")
			case '8':
				buf.WriteString("{{index . 8}}")
			case '9':
				buf.WriteString("{{index . 9}}")
			default:
				buf.WriteByte('\\')
				buf.WriteRune(ch)
			}
			state = stateReady

		default:
			panic(fmt.Errorf("invalid state %d", state))
		}
	}

	return buf.String()
}
