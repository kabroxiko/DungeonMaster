package llm

// ParseYAMLRoot parses YAML to a mapping or sequence (campaign stages may return bare lists).
func ParseYAMLRoot(raw string) (interface{}, bool) {
	return parseStructuredYAML(raw)
}

// ParseCampaignStageModelOutput mirrors server/utils/llmStructuredParse.js parseCampaignStageModelOutput.
func ParseCampaignStageModelOutput(raw string) (interface{}, bool) {
	return ParseYAMLRoot(raw)
}
