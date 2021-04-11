package main

type Config struct {
	Storage   *StorageConfig           `json:"storage"`
	Hosts     []string                 `json:"hosts"`
	MimeRules []*MimeRuleConfig        `json:"mimeRules"`
	Targets   map[string]*TargetConfig `json:"targets"`
	Rules     []*RuleConfig            `json:"rules"`
}

type StorageConfig struct {
	Engine  string   `json:"engine"`
	Path    string   `json:"path"`
	Servers []string `json:"servers"`
}

type MimeRuleConfig struct {
	Suffixes    []string `json:"suffixes"`
	ContentType string   `json:"contentType"`
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
