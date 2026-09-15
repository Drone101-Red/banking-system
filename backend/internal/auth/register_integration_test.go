//go:build integration

package auth_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"banking-system/internal/auth"
	"banking-system/internal/db"
	"banking-system/internal/models"
	"banking-system/internal/tigerbeetle"
)

// newTestEnv crea las dependencias reales para los tests de integración.
func newTestEnv(t *testing.T) (*auth.Service, *db.PostgresStore) {
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

	// Secret y expiry dummy: los tests de register no usan JWT.
	service := auth.NewService(pg, tbClient, "7t3ZZZ0nIVIS3Cb5zQP3S8x+ui0JjYUZ1nhCcimh3AM=", time.Hour)
	return service, pg
}

// uniqueEmail genera un email único por test para no chocar con otros.
func uniqueEmail(prefix string) string {
	return fmt.Sprintf("%s-%d@test.local", prefix, time.Now().UnixNano())
}

func TestRegister_HappyPath(t *testing.T) {
	service, _ := newTestEnv(t)
	ctx := context.Background()

	email := uniqueEmail("happy")
	user, err := service.Register(ctx, auth.RegisterRequest{
		Email:    email,
		Password: "TestPassword123!",
		FullName: "Juan Pérez",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if user.ID == "" {
		t.Fatal("user.ID vacío")
	}
	if user.Email != email {
		t.Fatalf("email = %s, esperado %s", user.Email, email)
	}
	if user.Status != models.StatusActive {
		t.Fatalf("status = %s, esperado ACTIVE", user.Status)
	}
	if len(user.TBAccountID) != 16 {
		t.Fatalf("TBAccountID len = %d, esperado 16", len(user.TBAccountID))
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	service, _ := newTestEnv(t)
	ctx := context.Background()

	email := uniqueEmail("dup")

	_, err := service.Register(ctx, auth.RegisterRequest{
		Email:    email,
		Password: "TestPassword123!",
		FullName: "Juan",
	})
	if err != nil {
		t.Fatalf("primer Register: %v", err)
	}

	_, err = service.Register(ctx, auth.RegisterRequest{
		Email:    email,
		Password: "OtherPassword123!",
		FullName: "Otro Juan",
	})
	if err == nil {
		t.Fatal("esperaba error por email duplicado")
	}
	if !strings.Contains(err.Error(), "EMAIL_EXISTS") {
		t.Fatalf("código esperado EMAIL_EXISTS, obtuve: %v", err)
	}
}

func TestRegister_InvalidEmail(t *testing.T) {
	service, _ := newTestEnv(t)
	ctx := context.Background()

	_, err := service.Register(ctx, auth.RegisterRequest{
		Email:    "noesunemail",
		Password: "TestPassword123!",
		FullName: "Juan",
	})
	if err == nil {
		t.Fatal("esperaba error por email inválido")
	}
	if !strings.Contains(err.Error(), "EMAIL_INVALID") {
		t.Fatalf("código esperado EMAIL_INVALID, obtuve: %v", err)
	}
}

func TestRegister_ShortPassword(t *testing.T) {
	service, _ := newTestEnv(t)
	ctx := context.Background()

	_, err := service.Register(ctx, auth.RegisterRequest{
		Email:    uniqueEmail("short"),
		Password: "short",
		FullName: "Juan",
	})
	if err == nil {
		t.Fatal("esperaba error por password corta")
	}
}

func TestRegister_EmailNormalization(t *testing.T) {
	service, pg := newTestEnv(t)
	ctx := context.Background()

	email := uniqueEmail("case")
	_, err := service.Register(ctx, auth.RegisterRequest{
		Email:    "  " + strings.ToUpper(email) + "  ",
		Password: "TestPassword123!",
		FullName: "Juan",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	user, err := pg.GetUserByEmail(ctx, email)
	if err != nil {
		t.Fatalf("no se encontró el usuario normalizado: %v", err)
	}
	if user.Email != email {
		t.Fatalf("email = %s, esperado %s", user.Email, email)
	}
}

func TestRegister_PersistsToPostgres(t *testing.T) {
	service, pg := newTestEnv(t)
	ctx := context.Background()

	email := uniqueEmail("persist")
	user, err := service.Register(ctx, auth.RegisterRequest{
		Email:    email,
		Password: "TestPassword123!",
		FullName: "Juan",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	fetched, err := pg.GetUserByEmail(ctx, email)
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if fetched.ID != user.ID {
		t.Fatalf("ID mismatch: %s vs %s", fetched.ID, user.ID)
	}
	if fetched.Status != models.StatusActive {
		t.Fatalf("status en PG = %s, esperado ACTIVE", fetched.Status)
	}
	if fetched.PasswordHash == "" {
		t.Fatal("password_hash vacío")
	}
	if fetched.PasswordHash == "TestPassword123!" {
		t.Fatal("password_hash no debería ser el plain")
	}
}
