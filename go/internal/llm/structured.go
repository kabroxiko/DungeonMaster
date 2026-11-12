package llm

import (
	"gopkg.in/yaml.v3"
)

// ParseModelStructuredObject mirrors utils/llmStructuredParse.js.
func ParseModelStructuredObject(raw string) (map[string]interface{}, bool) {
	doc, ok := parseStructuredYAML(raw)
	if !ok {
		return nil, false
	}
	m, ok := doc.(map[string]interface{})
	if !ok {
		return nil, false
	}
	return m, true
}

// parseStructuredYAML parses the first wire-format candidate that yields a document, so a reply only one repair can
// read is still read instead of being reported as unparseable.
func parseStructuredYAML(raw string) (interface{}, bool) {
	for _, candidate := range PrepareWireFormatCandidates(raw) {
		var doc interface{}
		if err := yaml.Unmarshal([]byte(candidate), &doc); err != nil {
			continue
		}
		if doc == nil {
			continue
		}
		return doc, true
	}
	return nil, false
}
