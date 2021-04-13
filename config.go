package main

type Config struct {
	Storage    *StorageConfig           `json:"storage"`
	Hosts      []string                 `json:"hosts"`
	ErrorPages *ErrorPagesConfig        `json:"errorPages"`
	IndexPages *IndexPagesConfig        `json:"indexPages"`
	MimeRules  []*MimeRuleConfig        `json:"mimeRules"`
	Targets    map[string]*TargetConfig `json:"targets"`
	Rules      []*RuleConfig            `json:"rules"`
}

type StorageConfig struct {
	Engine  string   `json:"engine"`
	Path    string   `json:"path"`
	Servers []string `json:"servers"`
}

type ErrorPagesConfig struct {
	Root        string            `json:"root"`
	Map         map[string]string `json:"map"`
	ContentType string            `json:"contentType"`
	ContentLang string            `json:"contentLanguage"`
}

type IndexPagesConfig struct {
	Path        string `json:"path"`
	ContentType string `json:"contentType"`
	ContentLang string `json:"contentLanguage"`
}

type MimeRuleConfig struct {
	Suffixes    []string `json:"suffixes"`
	ContentType string   `json:"contentType"`
	ContentLang string   `json:"contentLanguage"`
}

type TargetConfig struct {
	Type TargetType `json:"type"`

	Path string `json:"path"`

	Protocol string `json:"protocol"`
	Address  string `json:"address"`
}

type RuleConfig struct {
	Match     map[string]string `json:"match"`
	Mutations []*MutationConfig `json:"mutations"`
	Target    string            `json:"target"`
}

type MutationConfig struct {
	Type    MutationType `json:"type"`
	Header  string       `json:"header"`
	Search  string       `json:"search"`
	Replace string       `json:"replace"`
}
