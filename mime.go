package main

import (
	"net/http"
	"regexp"
	"strings"

	zerolog "github.com/rs/zerolog"
)

type MimeRule struct {
	rx          *regexp.Regexp
	contentType string
	contentLang string
}

func CompileMimeRule(cfg *MimeRuleConfig) (*MimeRule, error) {
	pieces := make([]string, len(cfg.Suffixes))
	for i, suffix := range cfg.Suffixes {
		pieces[i] = regexp.QuoteMeta(suffix)
	}

	pattern := `^.*(?:` + strings.Join(pieces, `|`) + `)$`
	rx := regexp.MustCompile(pattern)
	return &MimeRule{
		rx:          rx,
		contentType: cfg.ContentType,
		contentLang: cfg.ContentLang,
	}, nil
}

func DetectMimeProperties(impl *Impl, logger *zerolog.Logger, filesystem http.FileSystem, path string) (contentType string, contentLang string) {
	contentType = "application/octet-stream"
	contentLang = ""

	f, err := filesystem.Open(path)
	if err != nil {
		logger.Error().Str("file", path).Err(err).Msg("failed to open file")
		return
	}

	defer f.Close()

	haveContentType := false
	if raw, err := readXattr(f, xattrMimeType); err == nil {
		contentType = string(raw)
		haveContentType = true
	}

	haveContentLang := false
	if raw, err := readXattr(f, xattrMimeLang); err == nil {
		contentLang = string(raw)
		haveContentLang = true
	}

	for _, mimeRule := range impl.mimeRules {
		if !mimeRule.rx.MatchString(path) {
			continue
		}
		if !haveContentType && mimeRule.contentType != "" {
			contentType = mimeRule.contentType
			haveContentType = true
		}
		if !haveContentLang && mimeRule.contentLang != "" {
			contentLang = mimeRule.contentLang
			haveContentLang = true
		}
	}

	return
}
