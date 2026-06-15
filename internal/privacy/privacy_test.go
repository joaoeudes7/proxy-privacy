package privacy

import (
	"testing"

	"github.com/joaoeudes7/proxy-privacy/internal/config"
)

func TestBuildProvider_Strict(t *testing.T) {
	p := BuildProvider(config.PrivacyStrict)

	if p["data_collection"] != "deny" {
		t.Errorf("expected data_collection=deny, got %v", p["data_collection"])
	}
	if p["zdr"] != true {
		t.Errorf("expected zdr=true, got %v", p["zdr"])
	}
	if p["allow_fallbacks"] != false {
		t.Errorf("expected allow_fallbacks=false, got %v", p["allow_fallbacks"])
	}
	if _, ok := p["allow_fallbacks"]; !ok {
		t.Error("expected allow_fallbacks key to exist")
	}
	if _, ok := p["zdr"]; !ok {
		t.Error("expected zdr key to exist")
	}
}

func TestBuildProvider_Standard(t *testing.T) {
	p := BuildProvider(config.PrivacyStandard)

	if p["data_collection"] != "deny" {
		t.Errorf("expected data_collection=deny, got %v", p["data_collection"])
	}
	if p["allow_fallbacks"] != true {
		t.Errorf("expected allow_fallbacks=true, got %v", p["allow_fallbacks"])
	}
	if _, ok := p["zdr"]; ok {
		t.Error("expected zdr not to be present in standard mode")
	}
}

func TestBuildProvider_DefaultEmpty(t *testing.T) {
	p := BuildProvider("")

	if p["data_collection"] != "deny" {
		t.Errorf("expected data_collection=deny, got %v", p["data_collection"])
	}
	if p["allow_fallbacks"] != true {
		t.Errorf("expected allow_fallbacks=true, got %v", p["allow_fallbacks"])
	}
	if _, ok := p["zdr"]; ok {
		t.Error("expected zdr not to be present for empty mode")
	}
}

func TestRedactPII_Email(t *testing.T) {
	input := "Contact me at john.doe@example.com or support@test.org.uk"
	want := "Contact me at [REDACTED] or [REDACTED]"
	got := RedactPII(input)
	if got != want {
		t.Errorf("RedactPII(%q) = %q, want %q", input, got, want)
	}
}

func TestRedactPII_NoEmail(t *testing.T) {
	input := "This is plain text with no email addresses"
	got := RedactPII(input)
	if got != input {
		t.Errorf("RedactPII(%q) = %q, want unchanged", input, got)
	}
}

func TestRedactPII_EmptyString(t *testing.T) {
	got := RedactPII("")
	if got != "" {
		t.Errorf("RedactPII('') = %q, want ''", got)
	}
}

func TestRedactPII_EmailAtStart(t *testing.T) {
	input := "user@example.com is my email"
	want := "[REDACTED] is my email"
	got := RedactPII(input)
	if got != want {
		t.Errorf("RedactPII(%q) = %q, want %q", input, got, want)
	}
}

func TestRedactContent_String(t *testing.T) {
	got := redactContent("contact me at test@foo.com", true, false)
	want := "contact me at [REDACTED]"
	if got != want {
		t.Errorf("redactContent(string) = %v, want %v", got, want)
	}
}

func TestRedactContent_NonString(t *testing.T) {
	tests := []struct {
		name string
		in   any
	}{
		{"int", 42},
		{"float", 3.14},
		{"bool", true},
		{"nil", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := redactContent(tc.in, true, false)
			if got != tc.in {
				t.Errorf("redactContent(%v) = %v, want %v", tc.in, got, tc.in)
			}
		})
	}
}

func TestRedactContent_ArrayOfStrings(t *testing.T) {
	input := []any{"alice@foo.com", "bob@bar.com", "hello"}
	got := redactContent(input, true, false)
	want := []any{"[REDACTED]", "[REDACTED]", "hello"}
	for i := range want {
		if got.([]any)[i] != want[i] {
			t.Errorf("index %d: got %v, want %v", i, got.([]any)[i], want[i])
		}
	}
}

func TestRedactContent_ArrayMixed(t *testing.T) {
	input := []any{
		"user@example.com",
		42,
		map[string]any{"email": "nested@test.com"},
		[]any{"deep@inside.com"},
	}
	got := redactContent(input, true, false).([]any)

	if got[0] != "[REDACTED]" {
		t.Errorf("got[0] = %v, want [REDACTED]", got[0])
	}
	if got[1] != 42 {
		t.Errorf("got[1] = %v, want 42", got[1])
	}
	nestedMap := got[2].(map[string]any)
	if nestedMap["email"] != "[REDACTED]" {
		t.Errorf("nested map email = %v, want [REDACTED]", nestedMap["email"])
	}
	nestedArr := got[3].([]any)
	if nestedArr[0] != "[REDACTED]" {
		t.Errorf("nested arr[0] = %v, want [REDACTED]", nestedArr[0])
	}
}

func TestRedactContent_Map(t *testing.T) {
	input := map[string]any{
		"name":  "john",
		"email": "john@doe.com",
		"nested": map[string]any{
			"email": "deep@test.com",
		},
		"count": 10,
	}
	got := redactContent(input, true, false).(map[string]any)

	if got["name"] != "john" {
		t.Errorf("name = %v, want john", got["name"])
	}
	if got["email"] != "[REDACTED]" {
		t.Errorf("email = %v, want [REDACTED]", got["email"])
	}
	if got["count"] != 10 {
		t.Errorf("count = %v, want 10", got["count"])
	}
	nested := got["nested"].(map[string]any)
	if nested["email"] != "[REDACTED]" {
		t.Errorf("nested email = %v, want [REDACTED]", nested["email"])
	}
}

func TestRedactContent_ContentBlock(t *testing.T) {
	input := []any{
		map[string]any{
			"type": "text",
			"text": "user@example.com",
		},
	}
	got := redactContent(input, true, false).([]any)
	block := got[0].(map[string]any)
	if block["type"] != "text" {
		t.Errorf("type = %v, want text", block["type"])
	}
	if block["text"] != "[REDACTED]" {
		t.Errorf("text = %v, want [REDACTED]", block["text"])
	}
}
func TestRedactContent_EmptyArray(t *testing.T) {
	input := []any{}
	got := redactContent(input, true, false)
	if len(got.([]any)) != 0 {
		t.Errorf("expected empty array, got %v", got)
	}
}

func TestRedactContent_EmptyMap(t *testing.T) {
	input := map[string]any{}
	got := redactContent(input, true, false)
	if len(got.(map[string]any)) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestApply_ProviderAndRedactPII(t *testing.T) {
	body := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "Email me at alice@example.com",
			},
		},
	}
	result := Apply(body, config.PrivacyStandard, true, true)

	provider := result["provider"].(map[string]any)
	if provider["data_collection"] != "deny" {
		t.Errorf("expected data_collection=deny, got %v", provider["data_collection"])
	}
	if provider["allow_fallbacks"] != true {
		t.Errorf("expected allow_fallbacks=true, got %v", provider["allow_fallbacks"])
	}

	msgs := result["messages"].([]any)
	msg := msgs[0].(map[string]any)
	if msg["content"] != "Email me at [REDACTED]" {
		t.Errorf("content = %q, want %q", msg["content"], "Email me at [REDACTED]")
	}
	if msg["role"] != "user" {
		t.Errorf("role = %v, want user", msg["role"])
	}
}

func TestApply_StrictMode(t *testing.T) {
	body := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "hello",
			},
		},
	}
	result := Apply(body, config.PrivacyStrict, true, true)

	provider := result["provider"].(map[string]any)
	if provider["data_collection"] != "deny" {
		t.Errorf("expected data_collection=deny, got %v", provider["data_collection"])
	}
	if provider["zdr"] != true {
		t.Errorf("expected zdr=true, got %v", provider["zdr"])
	}
	if provider["allow_fallbacks"] != false {
		t.Errorf("expected allow_fallbacks=false, got %v", provider["allow_fallbacks"])
	}
}

func TestApply_RedactFalse(t *testing.T) {
	body := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "Email me at alice@example.com",
			},
		},
	}
	result := Apply(body, config.PrivacyStandard, false, false)

	msgs := result["messages"].([]any)
	msg := msgs[0].(map[string]any)
	if msg["content"] != "Email me at alice@example.com" {
		t.Errorf("content should be unchanged when redact=false, got %q", msg["content"])
	}
}

func TestApply_EmptyMessages(t *testing.T) {
	body := map[string]any{
		"messages": []any{},
	}
	result := Apply(body, config.PrivacyStandard, true, true)

	provider := result["provider"].(map[string]any)
	if provider["data_collection"] != "deny" {
		t.Errorf("expected data_collection=deny, got %v", provider["data_collection"])
	}
	if len(result["messages"].([]any)) != 0 {
		t.Errorf("expected empty messages, got %v", result["messages"])
	}
}

func TestApply_NestedContentBlocks(t *testing.T) {
	body := map[string]any{
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type": "text",
						"text": "user@example.com",
					},
				},
			},
		},
	}
	result := Apply(body, config.PrivacyStandard, true, true)

	msgs := result["messages"].([]any)
	msg := msgs[0].(map[string]any)
	content := msg["content"].([]any)
	block := content[0].(map[string]any)
	if block["type"] != "text" {
		t.Errorf("type = %v, want text", block["type"])
	}
	if block["text"] != "[REDACTED]" {
		t.Errorf("text = %v, want [REDACTED]", block["text"])
	}
}

func TestApply_NoMessagesKey(t *testing.T) {
	body := map[string]any{}
	result := Apply(body, config.PrivacyStandard, true, true)

	if _, ok := result["provider"]; !ok {
		t.Error("expected provider key to exist")
	}
}

func TestApply_MessagesNotArray(t *testing.T) {
	body := map[string]any{
		"messages": "not an array",
	}
	result := Apply(body, config.PrivacyStandard, true, true)

	if result["messages"] != "not an array" {
		t.Errorf("messages should be unchanged when not []any, got %v", result["messages"])
	}
}

func TestApply_PreservesOtherKeys(t *testing.T) {
	body := map[string]any{
		"model": "gpt-4",
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "hi",
			},
		},
		"temperature": 0.7,
	}
	result := Apply(body, config.PrivacyStandard, false, false)

	if result["model"] != "gpt-4" {
		t.Errorf("model = %v, want gpt-4", result["model"])
	}
	if result["temperature"] != 0.7 {
		t.Errorf("temperature = %v, want 0.7", result["temperature"])
	}
}

func TestRedactSecrets_OpenAIKey(t *testing.T) {
	input := "my key is sk-proj-abc123def456ghijklmnopqrstuv"
	want := "my key is [REDACTED]"
	got := RedactSecrets(input)
	if got != want {
		t.Errorf("RedactSecrets(%q) = %q, want %q", input, got, want)
	}
}

func TestRedactSecrets_AnthropicKey(t *testing.T) {
	input := "key=sk-ant-abcdef1234567890abcdef1234567890"
	want := "key=[REDACTED]"
	got := RedactSecrets(input)
	if got != want {
		t.Errorf("RedactSecrets(%q) = %q, want %q", input, got, want)
	}
}

func TestRedactSecrets_GitHubToken(t *testing.T) {
	input := "token: ghp_abcdef1234567890abcdef1234567890abcdefghij"
	want := "token: [REDACTED]"
	got := RedactSecrets(input)
	if got != want {
		t.Errorf("RedactSecrets(%q) = %q, want %q", input, got, want)
	}
}

func TestRedactSecrets_AWSKey(t *testing.T) {
	input := "AKIA1234567890ABCDEF"
	want := "[REDACTED]"
	got := RedactSecrets(input)
	if got != want {
		t.Errorf("RedactSecrets(%q) = %q, want %q", input, got, want)
	}
}

func TestRedactSecrets_NoSecret(t *testing.T) {
	input := "This is just regular text content"
	got := RedactSecrets(input)
	if got != input {
		t.Errorf("RedactSecrets(%q) = %q, want unchanged", input, got)
	}
}

func TestRedactSecrets_MultipleSecrets(t *testing.T) {
	input := "sk-proj-abc123def456ghijklmnopqrstuv and AKIA1234567890ABCDEF"
	want := "[REDACTED] and [REDACTED]"
	got := RedactSecrets(input)
	if got != want {
		t.Errorf("RedactSecrets(%q) = %q, want %q", input, got, want)
	}
}

func TestApply_SecretsOnly(t *testing.T) {
	body := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "Use sk-proj-abc123def456ghijklmnopqrstuv and email me at user@example.com",
			},
		},
	}
	result := Apply(body, config.PrivacyStandard, false, true)

	msgs := result["messages"].([]any)
	msg := msgs[0].(map[string]any)
	content := msg["content"].(string)

	if content != "Use [REDACTED] and email me at user@example.com" {
		t.Errorf("content = %q, want secrets redacted but emails preserved", content)
	}
}

func TestApply_PIIOnly(t *testing.T) {
	body := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "Use sk-proj-abc123def456ghijklmnopqrstuv and email me at user@example.com",
			},
		},
	}
	result := Apply(body, config.PrivacyStandard, true, false)

	msgs := result["messages"].([]any)
	msg := msgs[0].(map[string]any)
	content := msg["content"].(string)

	if content != "Use sk-proj-abc123def456ghijklmnopqrstuv and email me at [REDACTED]" {
		t.Errorf("content = %q, want emails redacted but secrets preserved", content)
	}
}

func TestApply_Both(t *testing.T) {
	body := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "Use sk-proj-abc123def456ghijklmnopqrstuv and email me at user@example.com",
			},
		},
	}
	result := Apply(body, config.PrivacyStandard, true, true)

	msgs := result["messages"].([]any)
	msg := msgs[0].(map[string]any)
	content := msg["content"].(string)

	if content != "Use [REDACTED] and email me at [REDACTED]" {
		t.Errorf("content = %q, want both redacted", content)
	}
}

func TestApply_Neither(t *testing.T) {
	body := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "Use sk-proj-abc123def456ghijklmnopqrstuv and email me at user@example.com",
			},
		},
	}
	result := Apply(body, config.PrivacyStandard, false, false)

	msgs := result["messages"].([]any)
	msg := msgs[0].(map[string]any)
	content := msg["content"].(string)

	if content != "Use sk-proj-abc123def456ghijklmnopqrstuv and email me at user@example.com" {
		t.Errorf("content = %q, want unchanged", content)
	}
}
