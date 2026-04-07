package llm

import (
	"gopkg.in/yaml.v3"
)

// ParseModelStructuredObject mirrors utils/llmStructuredParse.js.
func ParseModelStructuredObject(raw string) (map[string]interface{}, bool) {
	prepared := PrepareWireFormatText(raw)
	var doc map[string]interface{}
	if err := yaml.Unmarshal([]byte(prepared), &doc); err != nil {
		return nil, false
	}
	if doc == nil {
		return nil, false
	}
	return doc, true
}
