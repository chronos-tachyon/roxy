package main

import (
	"github.com/chronos-tachyon/roxy/internal/enums"
	"github.com/chronos-tachyon/roxy/lib/mainutil"
)

type Config struct {
	Global  *GlobalConfig            `json:"global"`
	Hosts   []string                 `json:"hosts"`
	Targets map[string]*TargetConfig `json:"targets"`
	Rules   []*RuleConfig            `json:"rules"`
}

type GlobalConfig struct {
	MimeFile              string                    `json:"mimeFile"`
	ACMEDirectoryURL      string                    `json:"acmeDirectoryURL"`
	ACMERegistrationEmail string                    `json:"acmeRegistrationEmail"`
	ACMEUserAgent         string                    `json:"acmeUserAgent"`
	MaxCacheSize          int64                     `json:"maxCacheSize"`
	MaxComputeDigestSize  int64                     `json:"maxComputeDigestSize"`
	ZK                    mainutil.ZKConfig         `json:"zookeeper"`
	Etcd                  mainutil.EtcdConfig       `json:"etcd"`
	ATC                   mainutil.GRPCClientConfig `json:"atc"`
	Storage               *StorageConfig            `json:"storage"`
	Pages                 *PagesConfig              `json:"pages"`
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
	Type   enums.TargetType         `json:"type"`
	Path   string                   `json:"path,omitempty"`
	Target string                   `json:"target,omitempty"`
	TLS    mainutil.TLSClientConfig `json:"tls,omitempty"`
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

type MimeFile []*MimeRuleConfig

type MimeRuleConfig struct {
	Suffixes    []string `json:"suffixes"`
	ContentType string   `json:"contentType"`
	ContentLang string   `json:"contentLanguage"`
	ContentEnc  string   `json:"contentEncoding"`
}
