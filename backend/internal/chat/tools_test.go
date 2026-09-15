package chat

import (
	"testing"
)

// --- extractAmount ---

func TestExtractAmount_Valid(t *testing.T) {
	args := map[string]any{"amount_cents": float64(5000)}
	got, err := extractAmount(args)
	if err != nil {
		t.Fatalf("extractAmount: %v", err)
	}
	if got != 5000 {
		t.Fatalf("amount = %d, esperado 5000", got)
	}
}

func TestExtractAmount_Missing(t *testing.T) {
	args := map[string]any{}
	_, err := extractAmount(args)
	if err == nil {
		t.Fatal("esperaba error por campo faltante")
	}
}

func TestExtractAmount_Zero(t *testing.T) {
	args := map[string]any{"amount_cents": float64(0)}
	_, err := extractAmount(args)
	if err == nil {
		t.Fatal("esperaba error por amount=0")
	}
}

func TestExtractAmount_Negative(t *testing.T) {
	args := map[string]any{"amount_cents": float64(-100)}
	_, err := extractAmount(args)
	if err == nil {
		t.Fatal("esperaba error por amount negativo")
	}
}

func TestExtractAmount_NotANumber(t *testing.T) {
	args := map[string]any{"amount_cents": "5000"} // string, no float64
	_, err := extractAmount(args)
	if err == nil {
		t.Fatal("esperaba error por tipo incorrecto")
	}
}

func TestExtractAmount_TruncatesDecimal(t *testing.T) {
	// float64(5000.7) → int64(5000). Es aceptable.
	args := map[string]any{"amount_cents": float64(5000.7)}
	got, err := extractAmount(args)
	if err != nil {
		t.Fatalf("extractAmount: %v", err)
	}
	if got != 5000 {
		t.Fatalf("amount = %d, esperado 5000", got)
	}
}

// --- formatCents ---

func TestFormatCents(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "$0.00"},
		{1, "$0.01"},
		{100, "$1.00"},
		{1050, "$10.50"},
		{10000, "$100.00"},
		{123456, "$1234.56"},
		{-100, "-$1.00"},
		{-1050, "-$10.50"},
	}
	for _, c := range cases {
		got := formatCents(c.in)
		if got != c.want {
			t.Errorf("formatCents(%d) = %q, esperado %q", c.in, got, c.want)
		}
	}
}

// --- codeDescription ---

func TestCodeDescription(t *testing.T) {
	cases := []struct {
		in   uint16
		want string
	}{
		{1, "deposit"},
		{2, "withdrawal"},
		{3, "transfer"},
		{99, "unknown"},
	}
	for _, c := range cases {
		got := codeDescription(c.in)
		if got != c.want {
			t.Errorf("codeDescription(%d) = %q, esperado %q", c.in, got, c.want)
		}
	}
}

// --- Tools ---

func TestTools_Count(t *testing.T) {
	tools := Tools()
	if len(tools) != 5 {
		t.Fatalf("esperaba 5 tools, obtuve %d", len(tools))
	}
}

func TestTools_Names(t *testing.T) {
	tools := Tools()
	want := map[string]bool{
		"get_balance": false,
		"get_history": false,
		"deposit":     false,
		"withdraw":    false,
		"transfer":    false,
	}
	for _, tool := range tools {
		if _, ok := want[tool.Function.Name]; !ok {
			t.Errorf("tool inesperada: %s", tool.Function.Name)
		}
		want[tool.Function.Name] = true
	}
	for name, found := range want {
		if !found {
			t.Errorf("tool faltante: %s", name)
		}
	}
}

func TestTools_AllHaveDescription(t *testing.T) {
	for _, tool := range Tools() {
		if tool.Type != "function" {
			t.Errorf("tool %s: type = %s, esperado function", tool.Function.Name, tool.Type)
		}
		if tool.Function.Description == "" {
			t.Errorf("tool %s: sin descripción", tool.Function.Name)
		}
		if tool.Function.Parameters == nil {
			t.Errorf("tool %s: sin parámetros", tool.Function.Name)
		}
	}
}
