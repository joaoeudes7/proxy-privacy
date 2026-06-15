package privacy

import (
	"regexp"

	"github.com/joaoeudes7/proxy-privacy/internal/config"
)

var emailRE = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)

var (
	openAIKeyRE    = regexp.MustCompile(`sk-[a-zA-Z0-9-]{20,}`)
	anthropicKeyRE = regexp.MustCompile(`sk-ant-[a-zA-Z0-9]{20,}`)
	gitHubTokenRE  = regexp.MustCompile(`gh[pousr]_[a-zA-Z0-9]{36,}`)
	awsKeyRE       = regexp.MustCompile(`AKIA[0-9A-Z]{16}`)
)

func RedactSecrets(text string) string {
	result := openAIKeyRE.ReplaceAllString(text, "[REDACTED]")
	result = anthropicKeyRE.ReplaceAllString(result, "[REDACTED]")
	result = gitHubTokenRE.ReplaceAllString(result, "[REDACTED]")
	result = awsKeyRE.ReplaceAllString(result, "[REDACTED]")
	return result
}

func BuildProvider(mode config.PrivacyMode) map[string]any {
	switch mode {
	case config.PrivacyStrict:
		return map[string]any{
			"data_collection": "deny",
			"zdr":             true,
			"allow_fallbacks": false,
		}
	default:
		return map[string]any{
			"data_collection": "deny",
			"allow_fallbacks": true,
		}
	}
}

func RedactPII(text string) string {
	return emailRE.ReplaceAllString(text, "[REDACTED]")
}

func redactContent(content any, redactPII bool, redactSecrets bool) any {
	switch v := content.(type) {
	case string:
		result := v
		if redactSecrets {
			result = RedactSecrets(result)
		}
		if redactPII {
			result = RedactPII(result)
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = redactContent(item, redactPII, redactSecrets)
		}
		return result
	case map[string]any:
		result := make(map[string]any, len(v))
		for k, val := range v {
			result[k] = redactContent(val, redactPII, redactSecrets)
		}
		return result
	}
	return content
}

func Apply(body map[string]any, mode config.PrivacyMode, redactPII bool, redactSecrets bool) map[string]any {
	if body == nil {
		body = make(map[string]any)
	}
	providerParams := BuildProvider(mode)
	if existing, ok := body["provider"].(map[string]any); ok {
		for k, v := range providerParams {
			existing[k] = v
		}
		body["provider"] = existing
	} else {
		body["provider"] = providerParams
	}
	if redactPII || redactSecrets {
		if msgs, ok := body["messages"].([]any); ok {
			for _, m := range msgs {
				if msg, ok := m.(map[string]any); ok {
					if content, exists := msg["content"]; exists {
						msg["content"] = redactContent(content, redactPII, redactSecrets)
					}
				}
			}
		}
	}
	return body
}
