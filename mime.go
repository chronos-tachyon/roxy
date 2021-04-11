package main

import (
	"regexp"
	"strings"
)

type MimeRule struct {
	rx          *regexp.Regexp
	contentType string
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
	}, nil
}
