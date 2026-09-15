//go:build integration

package transactions_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"

	"banking-system/internal/db"
	"banking-system/internal/models"
	"banking-system/internal/password"
	"banking-system/internal/tigerbeetle"
	"banking-system/internal/transactions"
)

// --- Setup ---

type testEnv struct {
	pg  *db.PostgresStore
	tb  *tigerbeetle.Client
	svc *transactions.Service
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	pgDSN := os.Getenv("POSTGRES_TEST_DSN")
	if pgDSN == "" {
		pgDSN = "host=172.30.0.2 port=5432 user=banking_user password=secret123 dbname=banking sslmode=disable"
	}
	tbAddress := os.Getenv("TB_TEST_ADDRESS")
	if tbAddress == "" {
		tbAddress = "172.30.0.10:3000"
	}

	pg, err := db.NewPostgresStore(pgDSN)
	if err != nil {
		t.Fatalf("postgres: %v", err)
	}
	t.Cleanup(func() { pg.Close() })

	tbClient, err := tigerbeetle.New(tbAddress, "0")
	if err != nil {
		t.Fatalf("tigerbeetle: %v", err)
	}
	t.Cleanup(func() { tbClient.Close() })

	return &testEnv{
		pg:  pg,
		tb:  tbClient,
		svc: transactions.NewService(pg, tbClient),
	}
}

// createActiveUser crea un usuario + su cuenta en TB + lo marca ACTIVE.
func createActiveUser(t *testing.T, env *testEnv, prefix string) *models.User {
	t.Helper()
	ctx := context.Background()

	// Generar tb_account_id (16 bytes aleatorios, no 0 ni 1)
	var b [16]byte
	for {
		if _, err := rand.Read(b[:]); err != nil {
			t.Fatalf("rand.Read: %v", err)
		}
		allZero := true
		for _, x := range b {
			if x != 0 {
				allZero = false
				break
			}
		}
		if allZero {
			continue
		}
		break
	}

	hash, err := password.Hash("TestPassword123!")
	if err != nil {
		t.Fatalf("password.Hash: %v", err)
	}

	u := &models.User{
		Email:        fmt.Sprintf("%s-%d@test.local", prefix, time.Now().UnixNano()),
		PasswordHash: hash,
		FullName:     "Test User",
		TBAccountID:  b[:],
	}
	if err := env.pg.CreateUserPending(ctx, u); err != nil {
		t.Fatalf("CreateUserPending: %v", err)
	}

	tbID := tb.BytesToUint128(b)
	if err := env.tb.CreateAccount(tbID, models.CodeSavings); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	if err := env.pg.UpdateUserStatusActive(ctx, u.ID); err != nil {
		t.Fatalf("UpdateUserStatusActive: %v", err)
	}
	u.Status = models.StatusActive

	return u
}

// tbIDFromBytes convierte 16 bytes a Uint128.
func tbIDFromBytes(b []byte) tb.Uint128 {
	var arr [16]byte
	copy(arr[:], b)
	return tb.BytesToUint128(arr)
}

// --- Tests ---

func TestDeposit_Integration(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	user := createActiveUser(t, env, "dep")

	tx, err := env.svc.Deposit(ctx, user.ID, 10000, "")
	if err != nil {
		t.Fatalf("Deposit: %v", err)
	}
	if tx.ID == "" {
		t.Fatal("tx.ID vacío")
	}
	if tx.AmountCents != 10000 {
		t.Fatalf("amount = %d, esperado 10000", tx.AmountCents)
	}
	if tx.Code != models.TransferCodeDeposit {
		t.Fatalf("code = %d, esperado %d", tx.Code, models.TransferCodeDeposit)
	}

	balance, err := env.tb.GetBalance(tbIDFromBytes(user.TBAccountID))
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance != 10000 {
		t.Fatalf("balance = %d, esperado 10000", balance)
	}
}

func TestWithdraw_Integration(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	user := createActiveUser(t, env, "wd")

	if _, err := env.svc.Deposit(ctx, user.ID, 10000, ""); err != nil {
		t.Fatalf("Deposit: %v", err)
	}

	tx, err := env.svc.Withdraw(ctx, user.ID, 3000, "")
	if err != nil {
		t.Fatalf("Withdraw: %v", err)
	}
	if tx.Code != models.TransferCodeWithdrawal {
		t.Fatalf("code = %d, esperado %d", tx.Code, models.TransferCodeWithdrawal)
	}

	balance, err := env.tb.GetBalance(tbIDFromBytes(user.TBAccountID))
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance != 7000 {
		t.Fatalf("balance = %d, esperado 7000", balance)
	}
}

func TestWithdraw_InsufficientFunds_Integration(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	user := createActiveUser(t, env, "wdif")

	if _, err := env.svc.Deposit(ctx, user.ID, 1000, ""); err != nil {
		t.Fatalf("Deposit: %v", err)
	}

	_, err := env.svc.Withdraw(ctx, user.ID, 5000, "")
	if err == nil {
		t.Fatal("esperaba error por fondos insuficientes")
	}
	if !strings.Contains(err.Error(), "INSUFFICIENT_FUNDS") {
		t.Fatalf("código esperado INSUFFICIENT_FUNDS, obtuve: %v", err)
	}
}

func TestTransfer_Integration(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	alice := createActiveUser(t, env, "alice")
	bob := createActiveUser(t, env, "bob")

	if _, err := env.svc.Deposit(ctx, alice.ID, 10000, ""); err != nil {
		t.Fatalf("Deposit Alice: %v", err)
	}

	bobTBHex := hex.EncodeToString(bob.TBAccountID)
	tx, err := env.svc.Transfer(ctx, alice.ID, bobTBHex, 3000, "")
	if err != nil {
		t.Fatalf("Transfer: %v", err)
	}
	if tx.Code != models.TransferCodeUserTransfer {
		t.Fatalf("code = %d, esperado %d", tx.Code, models.TransferCodeUserTransfer)
	}

	aliceBalance, _ := env.tb.GetBalance(tbIDFromBytes(alice.TBAccountID))
	bobBalance, _ := env.tb.GetBalance(tbIDFromBytes(bob.TBAccountID))

	if aliceBalance != 7000 {
		t.Fatalf("alice balance = %d, esperado 7000", aliceBalance)
	}
	if bobBalance != 3000 {
		t.Fatalf("bob balance = %d, esperado 3000", bobBalance)
	}
}

func TestTransfer_SameAccount_Integration(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	alice := createActiveUser(t, env, "alice-self")

	if _, err := env.svc.Deposit(ctx, alice.ID, 10000, ""); err != nil {
		t.Fatalf("Deposit: %v", err)
	}

	aliceTBHex := hex.EncodeToString(alice.TBAccountID)
	_, err := env.svc.Transfer(ctx, alice.ID, aliceTBHex, 1000, "")
	if err == nil {
		t.Fatal("esperaba error por misma cuenta")
	}
	if !strings.Contains(err.Error(), "SAME_ACCOUNT") {
		t.Fatalf("código esperado SAME_ACCOUNT, obtuve: %v", err)
	}
}

func TestTransfer_DestNotFound_Integration(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	alice := createActiveUser(t, env, "alice-destnf")

	if _, err := env.svc.Deposit(ctx, alice.ID, 10000, ""); err != nil {
		t.Fatalf("Deposit: %v", err)
	}

	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}
	randomHex := hex.EncodeToString(random[:])

	_, err := env.svc.Transfer(ctx, alice.ID, randomHex, 1000, "")
	if err == nil {
		t.Fatal("esperaba error por cuenta destino inexistente")
	}
	if !strings.Contains(err.Error(), "DEST_NOT_FOUND") {
		t.Fatalf("código esperado DEST_NOT_FOUND, obtuve: %v", err)
	}
}

func TestHistory_Integration(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	user := createActiveUser(t, env, "hist")

	if _, err := env.svc.Deposit(ctx, user.ID, 10000, ""); err != nil {
		t.Fatalf("Deposit 1: %v", err)
	}
	if _, err := env.svc.Deposit(ctx, user.ID, 5000, ""); err != nil {
		t.Fatalf("Deposit 2: %v", err)
	}
	if _, err := env.svc.Withdraw(ctx, user.ID, 3000, ""); err != nil {
		t.Fatalf("Withdraw: %v", err)
	}

	result, err := env.svc.History(ctx, user.ID, 1, 10)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if result.Total != 3 {
		t.Fatalf("total = %d, esperado 3", result.Total)
	}
	if len(result.Transactions) != 3 {
		t.Fatalf("transacciones = %d, esperado 3", len(result.Transactions))
	}
	if result.Page != 1 || result.Limit != 10 || result.TotalPages != 1 {
		t.Fatalf("paginación incorrecta: page=%d limit=%d totalPages=%d",
			result.Page, result.Limit, result.TotalPages)
	}
}
