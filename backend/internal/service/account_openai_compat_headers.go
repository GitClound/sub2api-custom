package service

import (
	"encoding/json"
	"net/http"
	"strings"
)

var openAICompatRestrictedHeaders = map[string]struct{}{
	"connection":               {},
	"content-length":           {},
	"host":                     {},
	"sec-websocket-accept":     {},
	"sec-websocket-extensions": {},
	"sec-websocket-key":        {},
	"sec-websocket-protocol":   {},
	"sec-websocket-version":    {},
	"upgrade":                  {},
}

func (a *Account) GetOpenAICompatHeaders() map[string]string {
	if a == nil || !a.IsOpenAIApiKey() || a.Credentials == nil {
		return nil
	}
	return normalizeOpenAICompatHeaders(a.Credentials["headers"])
}

func normalizeOpenAICompatHeaders(raw any) map[string]string {
	switch value := raw.(type) {
	case map[string]string:
		return sanitizeOpenAICompatHeaders(value)
	case map[string]any:
		headers := make(map[string]string, len(value))
		for key, item := range value {
			headerKey := strings.TrimSpace(key)
			headerValue, ok := item.(string)
			if !ok {
				continue
			}
			headers[headerKey] = headerValue
		}
		return sanitizeOpenAICompatHeaders(headers)
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return nil
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
			return nil
		}
		return normalizeOpenAICompatHeaders(parsed)
	default:
		return nil
	}
}

func sanitizeOpenAICompatHeaders(raw map[string]string) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	headers := make(map[string]string, len(raw))
	for key, value := range raw {
		headerKey := strings.TrimSpace(key)
		headerValue := strings.TrimSpace(value)
		if headerKey == "" || headerValue == "" {
			continue
		}
		headers[headerKey] = headerValue
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}

func applyOpenAICompatHeaders(dst http.Header, headers map[string]string) {
	if dst == nil || len(headers) == 0 {
		return
	}
	for key, value := range headers {
		headerKey := strings.TrimSpace(key)
		headerValue := strings.TrimSpace(value)
		if headerKey == "" || headerValue == "" || isRestrictedOpenAICompatHeader(headerKey) {
			continue
		}
		dst.Set(headerKey, headerValue)
	}
}

func isRestrictedOpenAICompatHeader(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return true
	}
	if _, blocked := openAICompatRestrictedHeaders[lower]; blocked {
		return true
	}
	return strings.HasPrefix(lower, "sec-websocket-")
}
