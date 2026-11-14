package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

func normalizeAssistantContent(content interface{}) string {
	if content == nil {
		return ""
	}
	switch v := content.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return ""
		}
		return v
	case []interface{}:
		var parts []string
		for _, p := range v {
			parts = append(parts, assistantContentPartToString(p))
		}
		joined := strings.Join(parts, "")
		if strings.TrimSpace(joined) != "" {
			return joined
		}
		for _, p := range v {
			if sj := stringifyIfPlayerCharacterRoot(p); sj != "" {
				return sj
			}
		}
		return ""
	case map[string]interface{}:
		if t, ok := v["text"].(string); ok && strings.TrimSpace(t) != "" {
			return t
		}
		if t, ok := v["content"].(string); ok && strings.TrimSpace(t) != "" {
			return t
		}
		if sj := stringifyIfPlayerCharacterRoot(v); sj != "" {
			return sj
		}
		b, err := json.Marshal(v)
		if err == nil && len(b) > 2 {
			return string(b)
		}
	}
	return ""
}

func assistantContentPartToString(part interface{}) string {
	if part == nil {
		return ""
	}
	switch v := part.(type) {
	case string:
		return v
	case float64:
		return fmt.Sprint(v)
	case bool:
		return fmt.Sprint(v)
	case []interface{}:
		var ss []string
		for _, x := range v {
			ss = append(ss, assistantContentPartToString(x))
		}
		return strings.Join(ss, "")
	case map[string]interface{}:
		if isReasoningLikeSegment(v) {
			return ""
		}
		if t, ok := v["text"].(string); ok {
			return t
		}
		if t, ok := v["content"].(string); ok {
			return t
		}
		if t, ok := v["output"].(string); ok {
			return t
		}
		if t, ok := v["value"].(string); ok {
			return t
		}
		if d, ok := v["delta"].(map[string]interface{}); ok {
			if t, ok := d["content"].(string); ok {
				return t
			}
		}
		if m, ok := v["message"].(map[string]interface{}); ok {
			if t, ok := m["content"].(string); ok {
				return t
			}
		}
		if sj := stringifyIfPlayerCharacterRoot(v); sj != "" {
			return sj
		}
	}
	return ""
}

func isReasoningLikeSegment(part map[string]interface{}) bool {
	t := strings.ToLower(fmt.Sprint(part["type"]))
	return t == "reasoning" || t == "thinking" || t == "chain_of_thought"
}

// stripHarmonyChannels returns the text a harmony-format model actually answered with.
//
// gpt-oss and other harmony models write their turn as channels - an analysis the model thinks in, then a final
// answer - and some servers hand that whole transcript back as plain content instead of splitting it. The analysis is
// not the answer, so only what follows the final channel is kept, and the control tokens themselves carry no text.
func stripHarmonyChannels(s string) string {
	if !strings.Contains(s, "<|") {
		return s
	}
	const finalChannel = "<|channel|>final<|message|>"
	if i := strings.LastIndex(s, finalChannel); i >= 0 {
		s = s[i+len(finalChannel):]
	} else if strings.Contains(s, "<|channel|>") {
		// The turn ended inside the channel the model reasons in, so it never wrote an answer. Saying so is the point:
		// handing the reasoning back makes the caller fail later on an unreadable document instead.
		return ""
	}
	// A reply ends where its channel was closed, if the server left those tokens in place.
	for _, closer := range []string{"<|end|>", "<|return|>", "<|eot|>"} {
		if i := strings.Index(s, closer); i >= 0 {
			s = s[:i]
		}
	}
	return strings.TrimSpace(stripControlTokens(s))
}

// stripControlTokens drops every <|...|> token, which is a marker rather than something the model said.
func stripControlTokens(s string) string {
	if !strings.Contains(s, "<|") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if strings.HasPrefix(s[i:], "<|") {
			if end := strings.Index(s[i+2:], "|>"); end >= 0 {
				i += 2 + end + 2
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func stringifyIfPlayerCharacterRoot(obj interface{}) string {
	m, ok := obj.(map[string]interface{})
	if !ok {
		return ""
	}
	if _, has := m["playerCharacter"]; has {
		b, err := json.Marshal(m)
		if err != nil {
			return ""
		}
		return string(b)
	}
	return ""
}

func coerceAssistantOutputToString(raw interface{}) string {
	if raw == nil {
		return ""
	}
	if n := normalizeAssistantContent(raw); n != "" {
		return n
	}
	if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
		return s
	}
	b, err := json.Marshal(raw)
	if err == nil && len(b) > 2 {
		return string(b)
	}
	return ""
}
