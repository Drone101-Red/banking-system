//go:build integration

package chat_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"

	"banking-system/internal/account"
	"banking-system/internal/chat"
	"banking-system/internal/db"
	"banking-system/internal/models"
	"banking-system/internal/password"
	"banking-system/internal/tigerbeetle"
	"banking-system/internal/transactions"
)

// --- Setup ---

type testEnv struct {
	pg      *db.PostgresStore
	tb      *tigerbeetle.Client
	account *account.Service
	txn     *transactions.Service
	chatSvc *chat.Service
	confirm chat.ConfirmationStore
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

	accountSvc := account.NewService(pg, tbClient)
	txnSvc := transactions.NewService(pg, tbClient)
	confirmStore := chat.NewPostgresConfirmationStore(pg)

	// Cliente OpenRouter dummy (no se usa en estos tests).
	orClient := chat.NewOpenRouterClient("dummy-key", "openrouter/free")

	chatSvc := chat.NewService(pg, accountSvc, txnSvc, orClient, confirmStore)

	return &testEnv{
		pg:      pg,
		tb:      tbClient,
		account: accountSvc,
		txn:     txnSvc,
		chatSvc: chatSvc,
		confirm: confirmStore,
	}
}

// createActiveUser crea un usuario + cuenta TB + ACTIVE.
func createActiveUser(t *testing.T, env *testEnv, prefix string) *models.User {
	t.Helper()
	ctx := context.Background()

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

// savePending guarda una operación pendiente para test.
func savePending(
	t *testing.T,
	env *testEnv,
	userID, opType string,
	amountCents int64,
	toAccountID string,
) *chat.PendingOperation {
	t.Helper()
	ctx := context.Background()

	op := &chat.PendingOperation{
		Type:        opType,
		AmountCents: amountCents,
		ToAccountID: toAccountID,
		UserID:      userID,
	}
	if err := env.confirm.Save(ctx, op); err != nil {
		t.Fatalf("Save: %v", err)
	}
	return op
}

// --- Tests ---

func TestConfirm_Deposit(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	user := createActiveUser(t, env, "conf-dep")

	op := savePending(t, env, user.ID, "deposit", 5000, "")

	result, err := env.chatSvc.ConfirmOperation(ctx, user.ID, op.Token)
	if err != nil {
		t.Fatalf("ConfirmOperation: %v", err)
	}
	if !result.Success {
		t.Fatal("esperaba success=true")
	}
	if result.Operation != "deposit" {
		t.Fatalf("operation = %s, esperado deposit", result.Operation)
	}

	// Verificar saldo
	balance, err := env.account.Balance(ctx, user.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if balance != 5000 {
		t.Fatalf("balance = %d, esperado 5000", balance)
	}
}

func TestConfirm_Withdraw(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	user := createActiveUser(t, env, "conf-wd")

	// Depositar primero (sin confirmación, directo)
	if _, err := env.txn.Deposit(ctx, user.ID, 10000, ""); err != nil {
		t.Fatalf("Deposit: %v", err)
	}

	op := savePending(t, env, user.ID, "withdraw", 3000, "")

	_, err := env.chatSvc.ConfirmOperation(ctx, user.ID, op.Token)
	if err != nil {
		t.Fatalf("ConfirmOperation: %v", err)
	}

	balance, _ := env.account.Balance(ctx, user.ID)
	if balance != 7000 {
		t.Fatalf("balance = %d, esperado 7000", balance)
	}
}

func TestConfirm_Transfer(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	alice := createActiveUser(t, env, "conf-alice")
	bob := createActiveUser(t, env, "conf-bob")

	if _, err := env.txn.Deposit(ctx, alice.ID, 10000, ""); err != nil {
		t.Fatalf("Deposit Alice: %v", err)
	}

	bobTBHex := hex.EncodeToString(bob.TBAccountID)
	op := savePending(t, env, alice.ID, "transfer", 3000, bobTBHex)

	_, err := env.chatSvc.ConfirmOperation(ctx, alice.ID, op.Token)
	if err != nil {
		t.Fatalf("ConfirmOperation: %v", err)
	}

	aliceBal, _ := env.account.Balance(ctx, alice.ID)
	bobBal, _ := env.account.Balance(ctx, bob.ID)

	if aliceBal != 7000 {
		t.Fatalf("alice = %d, esperado 7000", aliceBal)
	}
	if bobBal != 3000 {
		t.Fatalf("bob = %d, esperado 3000", bobBal)
	}
}

func TestConfirm_TokenUsed(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	user := createActiveUser(t, env, "conf-used")
	op := savePending(t, env, user.ID, "deposit", 5000, "")

	// Primera vez: OK
	if _, err := env.chatSvc.ConfirmOperation(ctx, user.ID, op.Token); err != nil {
		t.Fatalf("primera confirmación: %v", err)
	}

	// Segunda vez: debe fallar
	_, err := env.chatSvc.ConfirmOperation(ctx, user.ID, op.Token)
	if err == nil {
		t.Fatal("esperaba error al reusar token")
	}
}

func TestConfirm_WrongUser(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	alice := createActiveUser(t, env, "conf-owner")
	bob := createActiveUser(t, env, "conf-attacker")

	op := savePending(t, env, alice.ID, "deposit", 5000, "")

	// Bob intenta usar el token de Alice
	_, err := env.chatSvc.ConfirmOperation(ctx, bob.ID, op.Token)
	if err == nil {
		t.Fatal("esperaba error por token de otro usuario")
	}
}

func TestConfirm_InvalidToken(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	user := createActiveUser(t, env, "conf-invalid")

	_, err := env.chatSvc.ConfirmOperation(ctx, user.ID, "token-que-no-existe")
	if err == nil {
		t.Fatal("esperaba error por token inválido")
	}
}
