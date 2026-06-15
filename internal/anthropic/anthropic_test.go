package anthropic

import (
	"encoding/json"
	"testing"
)

func strPtr(s string) *string {
	return &s
}

func TestTranslateRequest_BasicTextContent(t *testing.T) {
	input := `{
		"model": "claude-sonnet-4",
		"messages": [{"role": "user", "content": "Hello"}],
		"max_tokens": 100,
		"stream": true
	}`
	out, err := TranslateRequest([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("invalid json output: %v", err)
	}
	if got["model"] != "claude-sonnet-4" {
		t.Errorf("model = %v, want claude-sonnet-4", got["model"])
	}
	if got["stream"] != true {
		t.Errorf("stream = %v, want true", got["stream"])
	}
	msgs, ok := got["messages"].([]any)
	if !ok || len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %v", msgs)
	}
	msg := msgs[0].(map[string]any)
	if msg["role"] != "user" {
		t.Errorf("role = %v, want user", msg["role"])
	}
	if msg["content"] != "Hello" {
		t.Errorf("content = %v, want Hello", msg["content"])
	}
}

func TestTranslateRequest_ContentBlocks(t *testing.T) {
	input := `{
		"model": "claude-sonnet-4",
		"messages": [{"role": "user", "content": [{"type": "text", "text": "Hello"}, {"type": "text", "text": "World"}]}],
		"max_tokens": 100
	}`
	out, err := TranslateRequest([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got map[string]any
	json.Unmarshal(out, &got)
	msgs := got["messages"].([]any)
	msg := msgs[0].(map[string]any)
	if msg["content"] != "Hello\nWorld" {
		t.Errorf("content = %q, want %q", msg["content"], "Hello\nWorld")
	}
}

func TestTranslateRequest_SystemAsString(t *testing.T) {
	input := `{
		"model": "claude-sonnet-4",
		"system": "You are helpful.",
		"messages": [{"role": "user", "content": "Hi"}],
		"max_tokens": 100
	}`
	out, err := TranslateRequest([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got map[string]any
	json.Unmarshal(out, &got)
	msgs := got["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	sys := msgs[0].(map[string]any)
	if sys["role"] != "system" || sys["content"] != "You are helpful." {
		t.Errorf("system message = %v", sys)
	}
	if msgs[1].(map[string]any)["role"] != "user" {
		t.Error("second message should be user")
	}
}

func TestTranslateRequest_SystemAsBlocks(t *testing.T) {
	input := `{
		"model": "claude-sonnet-4",
		"system": [{"type": "text", "text": "Be concise."}, {"type": "text", "text": "Use facts."}],
		"messages": [{"role": "user", "content": "Hi"}],
		"max_tokens": 100
	}`
	out, err := TranslateRequest([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got map[string]any
	json.Unmarshal(out, &got)
	msgs := got["messages"].([]any)
	sys := msgs[0].(map[string]any)
	want := "Be concise.\nUse facts."
	if sys["content"] != want {
		t.Errorf("system content = %q, want %q", sys["content"], want)
	}
}

func TestTranslateRequest_EmptyModelPreserved(t *testing.T) {
	input := `{"messages": [{"role": "user", "content": "Hi"}], "max_tokens": 100}`
	out, err := TranslateRequest([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got map[string]any
	json.Unmarshal(out, &got)
	if _, exists := got["model"]; !exists {
		t.Error("model should be present (even if empty)")
	}
}

func TestTranslateRequest_InvalidJSON(t *testing.T) {
	_, err := TranslateRequest([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestTranslateResponse_Basic(t *testing.T) {
	input := `{
		"id": "chatcmpl-abc123",
		"model": "gpt-4",
		"choices": [{"index": 0, "message": {"role": "assistant", "content": "Hello!"}, "finish_reason": "stop"}],
		"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
	}`
	out, err := TranslateResponse([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got map[string]any
	json.Unmarshal(out, &got)
	if got["id"] != "chatcmpl-abc123" {
		t.Errorf("id = %v", got["id"])
	}
	if got["type"] != "message" {
		t.Errorf("type = %v", got["type"])
	}
	if got["role"] != "assistant" {
		t.Errorf("role = %v", got["role"])
	}
	if got["model"] != "gpt-4" {
		t.Errorf("model = %v", got["model"])
	}
	content := got["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(content))
	}
	block := content[0].(map[string]any)
	if block["type"] != "text" || block["text"] != "Hello!" {
		t.Errorf("content block = %v", block)
	}
	if got["stop_reason"] != "end_turn" {
		t.Errorf("stop_reason = %v, want end_turn", got["stop_reason"])
	}
	usage, ok := got["usage"].(map[string]any)
	if !ok {
		t.Fatal("usage missing")
	}
	if usage["input_tokens"] != float64(10) || usage["output_tokens"] != float64(5) {
		t.Errorf("usage = %v", usage)
	}
}

func TestTranslateResponse_FinishReasonLength(t *testing.T) {
	input := `{"choices": [{"index": 0, "message": {"role": "assistant", "content": "..."}, "finish_reason": "length"}], "usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}}`
	out, err := TranslateResponse([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got map[string]any
	json.Unmarshal(out, &got)
	if got["stop_reason"] != "max_tokens" {
		t.Errorf("stop_reason = %v, want max_tokens", got["stop_reason"])
	}
}

func TestTranslateResponse_NoUsage(t *testing.T) {
	input := `{"choices": [{"index": 0, "message": {"role": "assistant", "content": "Hi"}}]}`
	out, err := TranslateResponse([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got map[string]any
	json.Unmarshal(out, &got)
	if _, exists := got["usage"]; exists {
		t.Error("usage should be omitted when input has none")
	}
}

func TestTranslateResponse_EmptyChoices(t *testing.T) {
	input := `{"choices": []}`
	out, err := TranslateResponse([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got map[string]any
	json.Unmarshal(out, &got)
	content := got["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(content))
	}
	block := content[0].(map[string]any)
	if block["text"] != "" {
		t.Errorf("expected empty text, got %q", block["text"])
	}
	if got["stop_reason"] != nil {
		t.Errorf("stop_reason should be nil when no finish_reason, got %v", got["stop_reason"])
	}
}

func TestTranslateResponse_InvalidJSON(t *testing.T) {
	_, err := TranslateResponse([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestStreamTranslator_FirstChunk(t *testing.T) {
	st := NewStreamTranslator()
	data := []byte(`{"id":"msg_123","object":"","model":"claude-3","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}`)
	events := st.Translate(data)
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(events))
	}
	if events[0].Event != "message_start" {
		t.Errorf("event[0] = %s, want message_start", events[0].Event)
	}
	var msgStart map[string]any
	json.Unmarshal(events[0].Data, &msgStart)
	if msgStart["type"] != "message_start" {
		t.Errorf("message_start type = %v", msgStart["type"])
	}
	msg, ok := msgStart["message"].(map[string]any)
	if !ok {
		t.Fatal("message_start missing message field")
	}
	if msg["id"] != "msg_123" {
		t.Errorf("message.id = %v, want msg_123", msg["id"])
	}
	if msg["model"] != "claude-3" {
		t.Errorf("message.model = %v, want claude-3", msg["model"])
	}

	if events[1].Event != "content_block_start" {
		t.Errorf("event[1] = %s, want content_block_start", events[1].Event)
	}

	hasDelta := false
	for _, e := range events {
		if e.Event == "content_block_delta" {
			hasDelta = true
			var d map[string]any
			json.Unmarshal(e.Data, &d)
			delta := d["delta"].(map[string]any)
			if delta["text"] != "Hello" {
				t.Errorf("delta text = %v, want Hello", delta["text"])
			}
		}
	}
	if !hasDelta {
		t.Error("expected content_block_delta event")
	}
}

func TestStreamTranslator_DeltaChunk(t *testing.T) {
	st := NewStreamTranslator()
	// First chunk to start
	st.Translate([]byte(`{"id":"msg_1","model":"m","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`))

	// Delta chunk
	events := st.Translate([]byte(`{"id":"msg_1","model":"m","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}`))
	if len(events) != 1 {
		t.Fatalf("expected 1 event (delta), got %d", len(events))
	}
	if events[0].Event != "content_block_delta" {
		t.Errorf("event = %s, want content_block_delta", events[0].Event)
	}
}

func TestStreamTranslator_DeltaSpecialChars(t *testing.T) {
	text := `He said "hello" and it's fine. Line1\nLine2\tTabbed\\Backslash\u00e9`

	st := NewStreamTranslator()
	st.Translate([]byte(`{"id":"msg_x","model":"m","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`))

	// Use a struct to guarantee field order matches the {"id":"... prefix requirement
	chunkBytes, _ := json.Marshal(struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			Index        int         `json:"index"`
			Delta        struct {
				Content string `json:"content"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
	}{
		ID:    "msg_x",
		Model: "m",
		Choices: []struct {
			Index        int         `json:"index"`
			Delta        struct {
				Content string `json:"content"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		}{
			{
				Index: 0,
				Delta: struct {
					Content string `json:"content"`
				}{Content: text},
				FinishReason: nil,
			},
		},
	})

	events := st.Translate(chunkBytes)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	var deltaData map[string]any
	if err := json.Unmarshal(events[0].Data, &deltaData); err != nil {
		t.Fatalf("invalid delta json: %v", err)
	}
	delta := deltaData["delta"].(map[string]any)
	got := delta["text"].(string)
	if got != text {
		t.Errorf("delta text = %q, want %q", got, text)
	}
}

func TestStreamTranslator_FinishReason(t *testing.T) {
	st := NewStreamTranslator()
	// First chunk
	st.Translate([]byte(`{"id":"msg_1","model":"m","choices":[{"index":0,"delta":{"role":"assistant","content":"Hi"},"finish_reason":null}]}`))

	// Chunk with finish_reason
	events := st.Translate([]byte(`{"id":"msg_1","model":"m","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":"stop"}]}`))
	if len(events) != 4 {
		t.Fatalf("expected 4 events (delta+stop+message_delta+message_stop), got %d: %+v", len(events), events)
	}
	if events[0].Event != "content_block_delta" {
		t.Errorf("events[0] = %s, want content_block_delta", events[0].Event)
	}
	if events[1].Event != "content_block_stop" {
		t.Errorf("events[1] = %s, want content_block_stop", events[1].Event)
	}
	if events[2].Event != "message_delta" {
		t.Errorf("events[2] = %s, want message_delta", events[2].Event)
	}
	if events[3].Event != "message_stop" {
		t.Errorf("events[3] = %s, want message_stop", events[3].Event)
	}

	var msgDelta map[string]any
	json.Unmarshal(events[2].Data, &msgDelta)
	delta := msgDelta["delta"].(map[string]any)
	if delta["stop_reason"] != "end_turn" {
		t.Errorf("stop_reason = %v, want end_turn", delta["stop_reason"])
	}
}

func TestStreamTranslator_FinishReasonLength(t *testing.T) {
	st := NewStreamTranslator()
	// Start
	st.Translate([]byte(`{"id":"m","model":"m","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`))
	events := st.Translate([]byte(`{"id":"m","model":"m","choices":[{"index":0,"delta":{"content":"a"},"finish_reason":"length"}]}`))
	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(events))
	}
	var msgDelta map[string]any
	json.Unmarshal(events[2].Data, &msgDelta)
	delta := msgDelta["delta"].(map[string]any)
	if delta["stop_reason"] != "max_tokens" {
		t.Errorf("stop_reason = %v, want max_tokens", delta["stop_reason"])
	}
}

func TestStreamTranslator_DONESentinel(t *testing.T) {
	st := NewStreamTranslator()
	// Start a stream
	st.Translate([]byte(`{"id":"msg_1","model":"m","choices":[{"index":0,"delta":{"role":"assistant","content":"Hi"},"finish_reason":null}]}`))

	events := st.Translate([]byte("[DONE]"))
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].Event != "content_block_stop" {
		t.Errorf("events[0] = %s, want content_block_stop", events[0].Event)
	}
	if events[1].Event != "message_delta" {
		t.Errorf("events[1] = %s, want message_delta", events[1].Event)
	}
	if events[2].Event != "message_stop" {
		t.Errorf("events[2] = %s, want message_stop", events[2].Event)
	}
}

func TestStreamTranslator_NonJSONLine(t *testing.T) {
	st := NewStreamTranslator()
	events := st.Translate([]byte(": keep-alive"))
	if events != nil {
		t.Error("expected nil for non-JSON lines")
	}
}

func TestStreamTranslator_InvalidJSON(t *testing.T) {
	st := NewStreamTranslator()
	events := st.Translate([]byte(`{invalid`))
	if events != nil {
		t.Error("expected nil for invalid json")
	}
}

func TestStreamTranslator_UsageInFinish(t *testing.T) {
	st := NewStreamTranslator()
	st.Translate([]byte(`{"id":"msg_1","model":"m","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`))
	events := st.Translate([]byte(`{"id":"msg_1","model":"m","choices":[{"index":0,"delta":{"content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`))

	var msgDelta map[string]any
	for _, e := range events {
		if e.Event == "message_delta" {
			json.Unmarshal(e.Data, &msgDelta)
		}
	}
	if msgDelta["usage"] == nil {
		t.Fatal("message_delta missing usage")
	}
	usage := msgDelta["usage"].(map[string]any)
	if usage["input_tokens"] != float64(10) || usage["output_tokens"] != float64(5) {
		t.Errorf("usage = %v, want input_tokens=10 output_tokens=5", usage)
	}
}

func TestFormatSSE(t *testing.T) {
	event := StreamEvent{Event: "message_start", Data: []byte(`{"type":"message_start"}`)}
	out := FormatSSE(event)
	want := "event: message_start\ndata: {\"type\":\"message_start\"}\n\n"
	if string(out) != want {
		t.Errorf("got %q, want %q", string(out), want)
	}
}

func TestExtractText_StringContent(t *testing.T) {
	raw := json.RawMessage(`"hello"`)
	got := extractText(raw)
	if got != "hello" {
		t.Errorf("extractText = %q, want %q", got, "hello")
	}
}

func TestExtractText_ArrayContent(t *testing.T) {
	raw := json.RawMessage(`[{"type":"text","text":"hello"},{"type":"text","text":"world"}]`)
	got := extractText(raw)
	if got != "hello\nworld" {
		t.Errorf("extractText = %q, want %q", got, "hello\nworld")
	}
}

func TestExtractText_Invalid(t *testing.T) {
	raw := json.RawMessage(`{"not": "valid"}`)
	got := extractText(raw)
	if got != "" {
		t.Errorf("extractText = %q, want empty string", got)
	}
}

func TestStreamTranslator_MultipleDeltasSameBlock(t *testing.T) {
	st := NewStreamTranslator()
	// Start
	st.Translate([]byte(`{"id":"msg_1","model":"m","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}`))
	// Delta 1
	e1 := st.Translate([]byte(`{"id":"msg_1","model":"m","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}`))
	// Delta 2
	e2 := st.Translate([]byte(`{"id":"msg_1","model":"m","choices":[{"index":0,"delta":{"content":"!"},"finish_reason":null}]}`))

	if len(e1) != 1 || e1[0].Event != "content_block_delta" {
		t.Error("delta 1 should produce content_block_delta")
	}
	if len(e2) != 1 || e2[0].Event != "content_block_delta" {
		t.Error("delta 2 should produce content_block_delta")
	}

	// No block_stop yet
	if st.blockStarted != true {
		t.Error("blockStarted should still be true")
	}
}

func TestTranslateRequest_PreservesStream(t *testing.T) {
	input := `{"model":"claude","messages":[{"role":"user","content":"hi"}],"max_tokens":10,"stream":true}`
	out, _ := TranslateRequest([]byte(input))
	var got map[string]any
	json.Unmarshal(out, &got)
	if got["stream"] != true {
		t.Errorf("stream = %v, want true", got["stream"])
	}

	// When stream=false, omitempty omits it from output
	input = `{"model":"claude","messages":[{"role":"user","content":"hi"}],"max_tokens":10,"stream":false}`
	out, _ = TranslateRequest([]byte(input))
	got2 := map[string]any{}
	json.Unmarshal(out, &got2)
	if _, exists := got2["stream"]; exists {
		t.Error("stream=false should be omitted due to omitempty")
	}
}
