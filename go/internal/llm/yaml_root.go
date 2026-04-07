package llm

import (
	"gopkg.in/yaml.v3"
)

// ParseYAMLRoot parses YAML to a mapping or sequence (campaign stages may return bare lists).
func ParseYAMLRoot(raw string) (interface{}, bool) {
	prepared := PrepareWireFormatText(raw)
	if prepared == "" {
		return nil, false
	}
	var doc interface{}
	if err := yaml.Unmarshal([]byte(prepared), &doc); err != nil {
		return nil, false
	}
	if doc == nil {
		return nil, false
	}
	return doc, true
}

// ParseCampaignStageModelOutput mirrors server/utils/llmStructuredParse.js parseCampaignStageModelOutput.
func ParseCampaignStageModelOutput(raw string) (interface{}, bool) {
	return ParseYAMLRoot(raw)
}
