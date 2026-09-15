package chat

import (
	"encoding/json"
	"testing"
)

// --- extractPendingConfirmation ---

func TestExtractPendingConfirmation_Valid_Int64(t *testing.T) {
	// Simula el resultado del Executor: int64 (creado en Go).
	result := map[string]any{
		"status":             "pending_confirmation",
		"confirmation_token": "abc-123",
		"type":               "deposit",
		"amount_cents":       int64(2500),
		"to_account_id":      "",
	}

	op := extractPendingConfirmation(result, "user-1")
	if op == nil {
		t.Fatal("esperaba PendingOperation, obtuve nil")
	}
	if op.Token != "abc-123" {
		t.Errorf("token = %s, esperado abc-123", op.Token)
	}
	if op.Type != "deposit" {
		t.Errorf("type = %s, esperado deposit", op.Type)
	}
	if op.AmountCents != 2500 {
		t.Errorf("amount = %d, esperado 2500", op.AmountCents)
	}
	if op.UserID != "user-1" {
		t.Errorf("userID = %s, esperado user-1", op.UserID)
	}
}

func TestExtractPendingConfirmation_Valid_Float64(t *testing.T) {
	// Simula el resultado deserializado de JSON: float64.
	result := map[string]any{
		"status":             "pending_confirmation",
		"confirmation_token": "xyz-789",
		"type":               "transfer",
		"amount_cents":       float64(5000),
		"to_account_id":      "4c5d789e69ae0ad5f3d630bc5931623e",
	}

	op := extractPendingConfirmation(result, "user-2")
	if op == nil {
		t.Fatal("esperaba PendingOperation, obtuve nil")
	}
	if op.AmountCents != 5000 {
		t.Errorf("amount = %d, esperado 5000", op.AmountCents)
	}
	if op.ToAccountID != "4c5d789e69ae0ad5f3d630bc5931623e" {
		t.Errorf("toAccountID incorrecto: %s", op.ToAccountID)
	}
}

func TestExtractPendingConfirmation_WrongStatus(t *testing.T) {
	result := map[string]any{
		"status": "completed",
	}
	if op := extractPendingConfirmation(result, "user-1"); op != nil {
		t.Fatal("esperaba nil para status distinto")
	}
}

func TestExtractPendingConfirmation_MissingToken(t *testing.T) {
	result := map[string]any{
		"status": "pending_confirmation",
		"type":   "deposit",
	}
	if op := extractPendingConfirmation(result, "user-1"); op != nil {
		t.Fatal("esperaba nil sin token")
	}
}

func TestExtractPendingConfirmation_NotAMap(t *testing.T) {
	if op := extractPendingConfirmation("not a map", "user-1"); op != nil {
		t.Fatal("esperaba nil para input no-map")
	}
	if op := extractPendingConfirmation(nil, "user-1"); op != nil {
		t.Fatal("esperaba nil para input nil")
	}
}

// --- extractInt64 ---

func TestExtractInt64_Int64(t *testing.T) {
	if got := extractInt64(int64(100)); got != 100 {
		t.Errorf("extractInt64(int64(100)) = %d", got)
	}
}

func TestExtractInt64_Float64(t *testing.T) {
	if got := extractInt64(float64(200)); got != 200 {
		t.Errorf("extractInt64(float64(200)) = %d", got)
	}
}

func TestExtractInt64_Int(t *testing.T) {
	if got := extractInt64(int(300)); got != 300 {
		t.Errorf("extractInt64(int(300)) = %d", got)
	}
}

func TestExtractInt64_JSONNumber(t *testing.T) {
	if got := extractInt64(json.Number("400")); got != 400 {
		t.Errorf("extractInt64(json.Number) = %d", got)
	}
}

func TestExtractInt64_Invalid(t *testing.T) {
	cases := []any{"string", true, nil, struct{}{}}
	for _, c := range cases {
		if got := extractInt64(c); got != 0 {
			t.Errorf("extractInt64(%v) = %d, esperado 0", c, got)
		}
	}
}

func TestExtractInt64_Float64Truncates(t *testing.T) {
	// 100.9 → 100
	if got := extractInt64(float64(100.9)); got != 100 {
		t.Errorf("extractInt64(100.9) = %d, esperado 100", got)
	}
}
