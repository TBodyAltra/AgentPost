package main

import (
	"encoding/json"
	"regexp"
	"strings"
)

var dashboardUnicodeEscapeRE = regexp.MustCompile(`\\u([0-9a-fA-F]{4})`)

// normalizeAgentMessageBodyForDisplay unwraps agent request/reply JSON and decodes
// literal \n / \uXXXX sequences so the dashboard can show human-readable text.
func normalizeAgentMessageBodyForDisplay(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	for depth := 0; depth < 4; depth++ {
		var parsed any
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			break
		}
		switch v := parsed.(type) {
		case string:
			next := strings.TrimSpace(v)
			if next == raw {
				return decodeLiteralEscapes(raw)
			}
			raw = next
			continue
		case map[string]any:
			if s, ok := v["request"].(string); ok {
				raw = strings.TrimSpace(s)
				continue
			}
			if s, ok := v["reply"].(string); ok {
				raw = strings.TrimSpace(s)
				continue
			}
			return raw
		default:
			return raw
		}
	}
	return decodeLiteralEscapes(raw)
}

func decodeLiteralEscapes(s string) string {
	if s == "" || !strings.Contains(s, `\`) {
		return s
	}
	s = strings.ReplaceAll(s, `\r\n`, "\n")
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\r`, "\n")
	s = strings.ReplaceAll(s, `\t`, "\t")
	s = dashboardUnicodeEscapeRE.ReplaceAllStringFunc(s, func(m string) string {
		sub := dashboardUnicodeEscapeRE.FindStringSubmatch(m)
		if len(sub) != 2 {
			return m
		}
		return string(rune(parseHexUint16(sub[1])))
	})
	return s
}

func parseHexUint16(hex string) uint16 {
	var n uint16
	for _, c := range hex {
		n <<= 4
		switch {
		case c >= '0' && c <= '9':
			n |= uint16(c - '0')
		case c >= 'a' && c <= 'f':
			n |= uint16(c-'a') + 10
		case c >= 'A' && c <= 'F':
			n |= uint16(c-'A') + 10
		}
	}
	return n
}
