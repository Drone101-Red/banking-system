//go:build integration

package tigerbeetle

import (
	"crypto/rand"
	"os"
	"testing"

	tb "github.com/tigerbeetle/tigerbeetle-go"

	"banking-system/internal/models"
)

// newTestClient crea un cliente conectado al TB de Docker Compose.
// Usa la variable de entorno TB_TEST_ADDRESS o el default del compose.
func newTestClient(t *testing.T) *Client {
	t.Helper()

	address := os.Getenv("TB_TEST_ADDRESS")
	if address == "" {
		address = "172.30.0.10:3000"
	}

	client, err := New(address, "0")
	if err != nil {
		t.Fatalf("no se pudo conectar a TB (%s): %v", address, err)
	}
	t.Cleanup(func() { client.Close() })

	return client
}

// randomAccountID genera un uint128 aleatorio no reservado (ni 0 ni 1).
func randomAccountID(t *testing.T) tb.Uint128 {
	t.Helper()

	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}

	id := tb.BytesToUint128(b)
	// Evitar 0 y 1 (reservados).
	if id == tb.ToUint128(0) || id == tb.ToUint128(1) {
		return randomAccountID(t)
	}
	return id
}

func TestCreateAccount_New(t *testing.T) {
	client := newTestClient(t)

	id := randomAccountID(t)
	if err := client.CreateAccount(id, models.CodeSavings); err != nil {
		t.Fatalf("CreateAccount (nueva): %v", err)
	}

	// Verificar que existe con la config correcta.
	accounts, err := client.client.LookupAccounts([]tb.Uint128{id})
	if err != nil {
		t.Fatalf("LookupAccounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("esperaba 1 cuenta, obtuve %d", len(accounts))
	}

	acc := accounts[0]
	if acc.Ledger != models.LedgerUSD {
		t.Errorf("Ledger = %d, esperado %d", acc.Ledger, models.LedgerUSD)
	}
	if acc.Code != models.CodeSavings {
		t.Errorf("Code = %d, esperado %d", acc.Code, models.CodeSavings)
	}

	expectedFlags := tb.AccountFlags{DebitsMustNotExceedCredits: true}.ToUint16()
	if acc.Flags != expectedFlags {
		t.Errorf("Flags = %d, esperado %d", acc.Flags, expectedFlags)
	}
}

func TestCreateAccount_Idempotent(t *testing.T) {
	client := newTestClient(t)

	id := randomAccountID(t)

	if err := client.CreateAccount(id, models.CodeSavings); err != nil {
		t.Fatalf("primera CreateAccount: %v", err)
	}
	if err := client.CreateAccount(id, models.CodeSavings); err != nil {
		t.Fatalf("segunda CreateAccount (idempotente): %v", err)
	}
}

func TestCreateAccount_ConfigMismatch_Code(t *testing.T) {
	client := newTestClient(t)

	id := randomAccountID(t)

	if err := client.CreateAccount(id, models.CodeSavings); err != nil {
		t.Fatalf("primera CreateAccount: %v", err)
	}

	// Intentar crear la misma cuenta con code distinto.
	err := client.CreateAccount(id, models.CodeBank)
	if err == nil {
		t.Fatal("esperaba error por code distinto, obtuve nil")
	}
}

// --- EnsureBankAccount ---

func TestEnsureBankAccount_Idempotent(t *testing.T) {
	client := newTestClient(t)

	// Puede existir o no; la primera llamada la crea o la encuentra.
	if err := client.EnsureBankAccount(); err != nil {
		t.Fatalf("primera EnsureBankAccount: %v", err)
	}
	// La segunda debe ser no-op.
	if err := client.EnsureBankAccount(); err != nil {
		t.Fatalf("segunda EnsureBankAccount (idempotente): %v", err)
	}
}

func TestEnsureBankAccount_Config(t *testing.T) {
	client := newTestClient(t)

	if err := client.EnsureBankAccount(); err != nil {
		t.Fatalf("EnsureBankAccount: %v", err)
	}

	bankID := tb.ToUint128(models.BankAccountID)
	accounts, err := client.client.LookupAccounts([]tb.Uint128{bankID})
	if err != nil {
		t.Fatalf("LookupAccounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("esperaba 1 cuenta banco, obtuve %d", len(accounts))
	}

	acc := accounts[0]
	if acc.Ledger != models.LedgerUSD {
		t.Errorf("bank Ledger = %d, esperado %d", acc.Ledger, models.LedgerUSD)
	}
	if acc.Code != models.CodeBank {
		t.Errorf("bank Code = %d, esperado %d", acc.Code, models.CodeBank)
	}
	if acc.Flags != 0 {
		t.Errorf("bank Flags = %d, esperado 0 (sin restricción de débito)", acc.Flags)
	}
}

func TestEnsureBankAccount_UnaffectedByUserAccounts(t *testing.T) {
	client := newTestClient(t)

	// Crear una cuenta de usuario cualquiera.
	userID := randomAccountID(t)
	if err := client.CreateAccount(userID, models.CodeSavings); err != nil {
		t.Fatalf("CreateAccount usuario: %v", err)
	}

	// EnsureBankAccount no debe verse afectado.
	if err := client.EnsureBankAccount(); err != nil {
		t.Fatalf("EnsureBankAccount tras crear usuario: %v", err)
	}

	// Verificar que ambas cuentas coexisten correctamente.
	bankID := tb.ToUint128(models.BankAccountID)
	accounts, err := client.client.LookupAccounts([]tb.Uint128{bankID, userID})
	if err != nil {
		t.Fatalf("LookupAccounts: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("esperaba 2 cuentas, obtuve %d", len(accounts))
	}
}
