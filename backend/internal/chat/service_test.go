package chat

import (
	"strings"
	"testing"
)

// --- buildInitialMessages ---

func TestBuildInitialMessages_Basic(t *testing.T) {
	s := &Service{}
	req := ChatRequestDTO{
		Message: "Hola",
	}
	msgs := s.buildInitialMessages(req)

	if len(msgs) != 2 {
		t.Fatalf("esperaba 2 mensajes (system + user), obtuve %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Errorf("msgs[0].Role = %s, esperado system", msgs[0].Role)
	}
	if msgs[0].Content == "" {
		t.Error("system prompt vacío")
	}
	if msgs[1].Role != "user" {
		t.Errorf("msgs[1].Role = %s, esperado user", msgs[1].Role)
	}
	if msgs[1].Content != "Hola" {
		t.Errorf("msgs[1].Content = %s, esperado Hola", msgs[1].Content)
	}
}

func TestBuildInitialMessages_WithHistory(t *testing.T) {
	s := &Service{}
	req := ChatRequestDTO{
		Message: "¿Y ahora?",
		History: []Message{
			{Role: "user", Content: "Hola"},
			{Role: "assistant", Content: "Hola, ¿en qué puedo ayudarte?"},
		},
	}
	msgs := s.buildInitialMessages(req)

	// system + 2 history + user = 4
	if len(msgs) != 4 {
		t.Fatalf("esperaba 4 mensajes, obtuve %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Errorf("msgs[0] no es system")
	}
	if msgs[1].Role != "user" || msgs[1].Content != "Hola" {
		t.Errorf("msgs[1] incorrecto: %+v", msgs[1])
	}
	if msgs[2].Role != "assistant" {
		t.Errorf("msgs[2] no es assistant")
	}
	if msgs[3].Role != "user" || msgs[3].Content != "¿Y ahora?" {
		t.Errorf("msgs[3] incorrecto: %+v", msgs[3])
	}
}

func TestBuildInitialMessages_TruncatesLongHistory(t *testing.T) {
	s := &Service{}

	// 30 mensajes de historial (más que maxHistoryMessages=20)
	history := make([]Message, 0, 30)
	for i := 0; i < 30; i++ {
		history = append(history, Message{
			Role:    "user",
			Content: "mensaje",
		})
	}

	req := ChatRequestDTO{
		Message: "nuevo",
		History: history,
	}
	msgs := s.buildInitialMessages(req)

	// system + 20 history + user = 22
	if len(msgs) != 1+maxHistoryMessages+1 {
		t.Fatalf("esperaba %d mensajes, obtuve %d", 1+maxHistoryMessages+1, len(msgs))
	}
}

// --- buildFallbackReply ---

func TestBuildFallbackReply_NoTools(t *testing.T) {
	reply := buildFallbackReply(nil)
	if reply == "" {
		t.Fatal("fallback vacío")
	}
	if !strings.Contains(reply, "reformular") {
		t.Errorf("fallback sin 'reformular': %s", reply)
	}
}

func TestBuildFallbackReply_WithTools(t *testing.T) {
	cases := []struct {
		toolName string
		want     string
	}{
		{"get_balance", "saldo"},
		{"get_history", "historial"},
		{"deposit", "Depósito"},
		{"withdraw", "Retiro"},
		{"transfer", "Transferencia"},
	}
	for _, c := range cases {
		tools := []ToolCall{
			{Function: ToolCallFunction{Name: c.toolName}},
		}
		reply := buildFallbackReply(tools)
		if !strings.Contains(reply, c.want) {
			t.Errorf("tool %s: reply = %q, esperado contener %q", c.toolName, reply, c.want)
		}
	}
}

func TestBuildFallbackReply_UnknownTool(t *testing.T) {
	tools := []ToolCall{
		{Function: ToolCallFunction{Name: "unknown_tool"}},
	}
	reply := buildFallbackReply(tools)
	if reply == "" {
		t.Fatal("fallback vacío para tool desconocida")
	}
}

// --- SystemPrompt ---

func TestSystemPrompt_ContainsRules(t *testing.T) {
	// El system prompt debe contener palabras clave críticas.
	keywords := []string{"español", "CENTAVOS", "confirmación"}
	for _, kw := range keywords {
		if !strings.Contains(SystemPrompt, kw) {
			t.Errorf("SystemPrompt no contiene %q", kw)
		}
	}
}
