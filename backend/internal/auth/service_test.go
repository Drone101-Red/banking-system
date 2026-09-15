package auth

import (
	"strings"
	"testing"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

func TestValidateRegister_Valid(t *testing.T) {
	req := RegisterRequest{
		Email:    "juan@example.com",
		Password: "TestPassword123!",
		FullName: "Juan Pérez",
	}
	if err := validateRegister(req, "juan@example.com", "Juan Pérez"); err != nil {
		t.Fatalf("validateRegister: %v", err)
	}
}

func TestValidateRegister_EmptyEmail(t *testing.T) {
	req := RegisterRequest{
		Password: "TestPassword123!",
		FullName: "Juan Pérez",
	}
	err := validateRegister(req, "", "Juan Pérez")
	if err == nil {
		t.Fatal("esperaba error por email vacío")
	}
	if !strings.Contains(err.Error(), "EMAIL_REQUIRED") {
		t.Fatalf("código esperado EMAIL_REQUIRED, obtuve: %v", err)
	}
}

func TestValidateRegister_InvalidEmail(t *testing.T) {
	req := RegisterRequest{
		Email:    "noesunemail",
		Password: "TestPassword123!",
		FullName: "Juan Pérez",
	}
	err := validateRegister(req, "noesunemail", "Juan Pérez")
	if err == nil {
		t.Fatal("esperaba error por email inválido")
	}
	if !strings.Contains(err.Error(), "EMAIL_INVALID") {
		t.Fatalf("código esperado EMAIL_INVALID, obtuve: %v", err)
	}
}

func TestValidateRegister_EmptyFullName(t *testing.T) {
	req := RegisterRequest{
		Email:    "juan@example.com",
		Password: "TestPassword123!",
	}
	err := validateRegister(req, "juan@example.com", "")
	if err == nil {
		t.Fatal("esperaba error por full_name vacío")
	}
	if !strings.Contains(err.Error(), "FULL_NAME_REQUIRED") {
		t.Fatalf("código esperado FULL_NAME_REQUIRED, obtuve: %v", err)
	}
}

func TestValidateRegister_ShortPassword(t *testing.T) {
	req := RegisterRequest{
		Email:    "juan@example.com",
		Password: "short",
		FullName: "Juan Pérez",
	}
	err := validateRegister(req, "juan@example.com", "Juan Pérez")
	if err == nil {
		t.Fatal("esperaba error por password corta")
	}
	if !strings.Contains(err.Error(), "PASSWORD_TOO_SHORT") {
		t.Fatalf("código esperado PASSWORD_TOO_SHORT, obtuve: %v", err)
	}
}

func TestValidateRegister_LongPassword(t *testing.T) {
	req := RegisterRequest{
		Email:    "juan@example.com",
		Password: strings.Repeat("a", 73),
		FullName: "Juan Pérez",
	}
	err := validateRegister(req, "juan@example.com", "Juan Pérez")
	if err == nil {
		t.Fatal("esperaba error por password larga")
	}
	if !strings.Contains(err.Error(), "PASSWORD_TOO_LONG") {
		t.Fatalf("código esperado PASSWORD_TOO_LONG, obtuve: %v", err)
	}
}

func TestGenerateTBAccountID_Valid(t *testing.T) {
	id, bytes, err := generateTBAccountID()
	if err != nil {
		t.Fatalf("generateTBAccountID: %v", err)
	}
	if id == tb.ToUint128(0) {
		t.Fatal("no debería generar 0")
	}
	if id == tb.ToUint128(1) {
		t.Fatal("no debería generar 1")
	}
	if len(bytes) != 16 {
		t.Fatalf("esperaba 16 bytes, obtuve %d", len(bytes))
	}
	if tb.BytesToUint128([16]byte(bytes)) != id {
		t.Fatal("bytes e id no coinciden")
	}
}

func TestGenerateTBAccountID_Randomness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id, _, err := generateTBAccountID()
		if err != nil {
			t.Fatalf("iteración %d: %v", i, err)
		}
		b := id.Bytes()
		key := string(b[:])
		if seen[key] {
			t.Fatalf("colisión en iteración %d", i)
		}
		seen[key] = true
	}
}

// --- validateLogin ---

func TestValidateLogin_Valid(t *testing.T) {
	req := LoginRequest{
		Email:    "juan@example.com",
		Password: "TestPassword123!",
	}
	if err := validateLogin(req, "juan@example.com"); err != nil {
		t.Fatalf("validateLogin: %v", err)
	}
}

func TestValidateLogin_EmptyEmail(t *testing.T) {
	req := LoginRequest{
		Password: "TestPassword123!",
	}
	err := validateLogin(req, "")
	if err == nil {
		t.Fatal("esperaba error por email vacío")
	}
	if !strings.Contains(err.Error(), "EMAIL_REQUIRED") {
		t.Fatalf("código esperado EMAIL_REQUIRED, obtuve: %v", err)
	}
}

func TestValidateLogin_EmptyPassword(t *testing.T) {
	req := LoginRequest{
		Email: "juan@example.com",
	}
	err := validateLogin(req, "juan@example.com")
	if err == nil {
		t.Fatal("esperaba error por password vacía")
	}
	if !strings.Contains(err.Error(), "PASSWORD_REQUIRED") {
		t.Fatalf("código esperado PASSWORD_REQUIRED, obtuve: %v", err)
	}
}

func TestValidateLogin_NoEmailFormatValidation(t *testing.T) {
	// validateLogin NO valida formato del email (eso se resuelve con el
	// lookup). Acepta emails mal formados; el login fallará con
	// INVALID_CREDENTIALS.
	req := LoginRequest{
		Email:    "noesunemail",
		Password: "TestPassword123!",
	}
	if err := validateLogin(req, "noesunemail"); err != nil {
		t.Fatalf("validateLogin no debería validar formato: %v", err)
	}
}
