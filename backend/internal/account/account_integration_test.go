//go:build integration

package account_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"testing"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"

	"banking-system/internal/account"
	"banking-system/internal/db"
	"banking-system/internal/models"
	"banking-system/internal/password"
	"banking-system/internal/tigerbeetle"
)

type testEnv struct {
	pg  *db.PostgresStore
	tb  *tigerbeetle.Client
	svc *account.Service
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
		svc: account.NewService(pg, tbClient),
	}
}

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

	ts := time.Now().UnixNano()
	alias := fmt.Sprintf("%s-%d", prefix, ts)
	if len(alias) > 50 {
		alias = alias[:50]
	}

	u := &models.User{
		Email:        fmt.Sprintf("%s-%d@test.local", prefix, ts),
		PasswordHash: hash,
		FullName:     "Test User",
		Alias:        alias,
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
func TestAccountInfo_Integration(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	user := createActiveUser(t, env, "acct-info")

	info, err := env.svc.Info(ctx, user.ID)
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Ledger != models.LedgerUSD {
		t.Fatalf("ledger = %d, esperado %d", info.Ledger, models.LedgerUSD)
	}
	if info.Code != models.CodeSavings {
		t.Fatalf("code = %d, esperado %d", info.Code, models.CodeSavings)
	}
	if info.BalanceCents != 0 {
		t.Fatalf("balance inicial = %d, esperado 0", info.BalanceCents)
	}
}

func TestAccountBalance_Integration(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	user := createActiveUser(t, env, "acct-bal")

	balance, err := env.svc.Balance(ctx, user.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if balance != 0 {
		t.Fatalf("balance inicial = %d, esperado 0", balance)
	}
}
