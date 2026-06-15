package anthropic

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

type StreamEvent struct {
	Event string
	Data  []byte
}

type MessageRequest struct {
	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	System    json.RawMessage `json:"system,omitempty"`
	MaxTokens int       `json:"max_tokens"`
	Stream    bool      `json:"stream,omitempty"`
}

type Message struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // string or []ContentBlock
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type OpenAIRequest struct {
	Model       string            `json:"model"`
	Messages    []OpenAIMessage   `json:"messages"`
	Stream      bool              `json:"stream,omitempty"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
}

type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func TranslateRequest(anthropicBody []byte) ([]byte, error) {
	var req MessageRequest
	if err := json.Unmarshal(anthropicBody, &req); err != nil {
		return nil, fmt.Errorf("invalid anthropic request: %w", err)
	}

	openai := OpenAIRequest{
		Model:     req.Model,
		Stream:    req.Stream,
		MaxTokens: req.MaxTokens,
	}

	if req.System != nil {
		systemText := extractSystemText(req.System)
		if systemText != "" {
			openai.Messages = append(openai.Messages, OpenAIMessage{
				Role:    "system",
				Content: systemText,
			})
		}
	}

	for _, msg := range req.Messages {
		content := extractText(msg.Content)
		openai.Messages = append(openai.Messages, OpenAIMessage{
			Role:    msg.Role,
			Content: content,
		})
	}

	return json.Marshal(openai)
}

func extractSystemText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []ContentBlock
	if json.Unmarshal(raw, &blocks) == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func extractText(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []ContentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

type OpenAIChoice struct {
	Index        int              `json:"index"`
	Message      OpenAIAssistMsg  `json:"message"`
	FinishReason *string          `json:"finish_reason"`
}

type OpenAIAssistMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type OpenAIResponse struct {
	ID      string          `json:"id"`
	Model   string          `json:"model"`
	Choices []OpenAIChoice  `json:"choices"`
	Usage   *OpenAIUsage    `json:"usage,omitempty"`
}

type AnthropicResponse struct {
	ID         string              `json:"id"`
	Type       string              `json:"type"`
	Role       string              `json:"role"`
	Content    []AnthropicContent  `json:"content"`
	Model      string              `json:"model"`
	StopReason *string             `json:"stop_reason"`
	Usage      *AnthropicUsage     `json:"usage,omitempty"`
}

type AnthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

func TranslateResponse(openaiBody []byte) ([]byte, error) {
	var oai OpenAIResponse
	if err := json.Unmarshal(openaiBody, &oai); err != nil {
		return nil, fmt.Errorf("invalid openai response: %w", err)
	}

	text := ""
	if len(oai.Choices) > 0 {
		text = oai.Choices[0].Message.Content
	}

	resp := AnthropicResponse{
		ID:   oai.ID,
		Type: "message",
		Role: "assistant",
		Content: []AnthropicContent{
			{Type: "text", Text: text},
		},
		Model: oai.Model,
	}

	if len(oai.Choices) > 0 && oai.Choices[0].FinishReason != nil {
		reason := *oai.Choices[0].FinishReason
		stopReason := mapFinishReason(reason)
		resp.StopReason = &stopReason
	}

	if oai.Usage != nil {
		resp.Usage = &AnthropicUsage{
			InputTokens:  oai.Usage.PromptTokens,
			OutputTokens: oai.Usage.CompletionTokens,
		}
	}

	return json.Marshal(resp)
}

func mapFinishReason(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "content_filter":
		return "content_filter"
	case "tool_calls":
		return "tool_use"
	default:
		return reason
	}
}

type StreamTranslator struct {
	mu         sync.Mutex
	started    bool
	blockStarted bool
	id         string
	model      string
}

func NewStreamTranslator() *StreamTranslator {
	return &StreamTranslator{}
}

type openaiChunk struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"`
	Model   string        `json:"model"`
	Choices []struct {
		Index        int     `json:"index"`
		Delta        struct {
			Role    string `json:"role,omitempty"`
			Content string `json:"content,omitempty"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *OpenAIUsage `json:"usage,omitempty"`
}

func (st *StreamTranslator) Translate(data []byte) []StreamEvent {
	line := strings.TrimSpace(string(data))

	if line == "[DONE]" {
		st.mu.Lock()
		defer st.mu.Unlock()
		var events []StreamEvent
		if st.blockStarted {
			events = append(events, StreamEvent{Event: "content_block_stop", Data: []byte(`{"type":"content_block_stop","index":0}`)})
		}
		events = append(events,
			StreamEvent{Event: "message_delta", Data: []byte(`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":0}}`)},
			StreamEvent{Event: "message_stop", Data: []byte(`{"type":"message_stop"}`)},
		)
		return events
	}

	if !strings.HasPrefix(line, "{") {
		return nil
	}

	var chunk openaiChunk
	if err := json.Unmarshal([]byte(line), &chunk); err != nil {
		return nil
	}

	if chunk.ID != "" {
		st.mu.Lock()
		st.id = chunk.ID
		st.model = chunk.Model
		st.mu.Unlock()
	}

	var events []StreamEvent

	st.mu.Lock()
	if !st.started {
		st.started = true
		st.mu.Unlock()
		msgStart := fmt.Sprintf(`{"type":"message_start","message":{"id":"%s","type":"message","role":"assistant","content":[],"model":"%s","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":0,"output_tokens":0}}}`, chunk.ID, chunk.Model)
		events = append(events, StreamEvent{Event: "message_start", Data: []byte(msgStart)})

		cbStart := fmt.Sprintf(`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		events = append(events, StreamEvent{Event: "content_block_start", Data: []byte(cbStart)})
		st.mu.Lock()
		st.blockStarted = true
		st.mu.Unlock()
	} else {
		st.mu.Unlock()
	}

	if len(chunk.Choices) == 0 {
		return events
	}

	delta := chunk.Choices[0].Delta.Content
	if delta != "" {
		deltaData := map[string]any{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]any{
				"type": "text_delta",
				"text": delta,
			},
		}
		deltaBytes, _ := json.Marshal(deltaData)
		events = append(events, StreamEvent{Event: "content_block_delta", Data: deltaBytes})
	}

	if chunk.Choices[0].FinishReason != nil {
		reason := *chunk.Choices[0].FinishReason
		stopReason := mapFinishReason(reason)
		st.mu.Lock()
		st.blockStarted = false
		st.mu.Unlock()
		events = append(events, StreamEvent{Event: "content_block_stop", Data: []byte(`{"type":"content_block_stop","index":0}`)})

		usage := `{"input_tokens":0,"output_tokens":0}`
		if chunk.Usage != nil {
			usage = fmt.Sprintf(`{"input_tokens":%d,"output_tokens":%d}`, chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens)
		}
		msgDelta := fmt.Sprintf(`{"type":"message_delta","delta":{"stop_reason":"%s","stop_sequence":null},"usage":%s}`, stopReason, usage)
		events = append(events, StreamEvent{Event: "message_delta", Data: []byte(msgDelta)})
		events = append(events, StreamEvent{Event: "message_stop", Data: []byte(`{"type":"message_stop"}`)})
	}

	return events
}

func FormatSSE(event StreamEvent) []byte {
	return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", event.Event, string(event.Data)))
}
