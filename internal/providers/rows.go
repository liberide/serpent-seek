package providers

import (
	"encoding/json"
	"strings"
)

// parseJSONObject decodes a JSON object. A JSON array or scalar counts as
// invalid (providers always answer with an object).
func parseJSONObject(body []byte) (map[string]any, bool) {
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, false
	}
	return data, true
}

// asString renders a JSON value as a trimmed string.
func asString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case nil:
		return ""
	default:
		return strings.TrimSpace(strings.Trim(strings.ReplaceAll(toJSON(t), `"`, ""), " \n\t"))
	}
}

func toJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// asInt renders a JSON number/string as an int.
func asInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case string:
		n := 0
		for _, r := range strings.TrimSpace(t) {
			if r < '0' || r > '9' {
				if n == 0 {
					return 0
				}
				break
			}
			n = n*10 + int(r-'0')
		}
		return n
	default:
		return 0
	}
}

// asBool renders a JSON bool, defaulting when the field is absent.
func asBool(v any, def bool) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "true", "1", "yes":
			return true
		case "false", "0", "no":
			return false
		}
	case float64:
		return t != 0
	}
	return def
}

// firstString returns the first non-empty string field.
func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s := asString(v); s != "" {
				return s
			}
		}
	}
	return ""
}

// toRow normalizes one result object into a Row, using provider-specific keys.
func toRow(m map[string]any, linkKeys []string, titleKey string, snippetKeys []string) (Row, bool) {
	link := firstString(m, linkKeys...)
	if link == "" {
		return Row{}, false
	}
	return Row{
		Link:    link,
		Title:   firstString(m, titleKey),
		Snippet: firstString(m, snippetKeys...),
	}, true
}

// collectRows converts a JSON array into normalized rows.
func collectRows(v any, linkKeys []string, titleKey string, snippetKeys []string) Rows {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make(Rows, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if row, ok := toRow(m, linkKeys, titleKey, snippetKeys); ok {
			out = append(out, row)
		}
	}
	return out
}
