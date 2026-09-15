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

// testJWTSecret es el secret dummy usado por los tests de integración.
// NO es seguro, es solo para tests. No usar en producción.
const testJWTSecret = "934d21db7326f4f86164171b2d523f33f0da6e64488a3efe8d542288767e3aa3"

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

	service := auth.NewService(pg, tbClient, testJWTSecret, time.Hour)
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

// --- LookupUser ---

func TestLookupUser_ByEmail(t *testing.T) {
	service, _ := newTestEnv(t)
	ctx := context.Background()

	email := uniqueEmail("lookup-email")
	user, err := service.Register(ctx, auth.RegisterRequest{
		Email:    email,
		Password: "TestPassword123!",
		FullName: "Lookup Test",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	result, err := service.LookupUser(ctx, email, "")
	if err != nil {
		t.Fatalf("LookupUser: %v", err)
	}
	if result.UserID != user.ID {
		t.Fatalf("user_id = %s, esperado %s", result.UserID, user.ID)
	}
	if result.Email != email {
		t.Fatalf("email = %s, esperado %s", result.Email, email)
	}
	if result.Alias == "" {
		t.Fatal("alias vacío")
	}
	if result.TBAccountID == "" {
		t.Fatal("tb_account_id vacío")
	}
}

func TestLookupUser_ByAlias(t *testing.T) {
	service, _ := newTestEnv(t)
	ctx := context.Background()

	email := uniqueEmail("lookup-alias")
	user, err := service.Register(ctx, auth.RegisterRequest{
		Email:    email,
		Password: "TestPassword123!",
		FullName: "Lookup Alias",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	result, err := service.LookupUser(ctx, "", user.Alias)
	if err != nil {
		t.Fatalf("LookupUser: %v", err)
	}
	if result.UserID != user.ID {
		t.Fatalf("user_id = %s, esperado %s", result.UserID, user.ID)
	}
	if result.Alias != user.Alias {
		t.Fatalf("alias = %s, esperado %s", result.Alias, user.Alias)
	}
}

func TestLookupUser_NotFound(t *testing.T) {
	service, _ := newTestEnv(t)
	ctx := context.Background()

	_, err := service.LookupUser(ctx, "noexiste@test.local", "")
	if err == nil {
		t.Fatal("esperaba error por usuario inexistente")
	}
	if !strings.Contains(err.Error(), "USER_NOT_FOUND") {
		t.Fatalf("código esperado USER_NOT_FOUND, obtuve: %v", err)
	}
}

func TestLookupUser_MissingQuery(t *testing.T) {
	service, _ := newTestEnv(t)
	ctx := context.Background()

	_, err := service.LookupUser(ctx, "", "")
	if err == nil {
		t.Fatal("esperaba error por query vacía")
	}
	if !strings.Contains(err.Error(), "MISSING_QUERY") {
		t.Fatalf("código esperado MISSING_QUERY, obtuve: %v", err)
	}
}

func TestLookupUser_AmbiguousQuery(t *testing.T) {
	service, _ := newTestEnv(t)
	ctx := context.Background()

	_, err := service.LookupUser(ctx, "a@test.local", "alias")
	if err == nil {
		t.Fatal("esperaba error por query ambigua")
	}
	if !strings.Contains(err.Error(), "AMBIGUOUS_QUERY") {
		t.Fatalf("código esperado AMBIGUOUS_QUERY, obtuve: %v", err)
	}
}

// --- Alias generation ---

func TestRegister_GeneratesAlias(t *testing.T) {
	service, _ := newTestEnv(t)
	ctx := context.Background()

	// Email único con timestamp para evitar colisiones entre corridas
	ts := time.Now().UnixNano()
	email := fmt.Sprintf("juangomez%d@example.com", ts)

	user, err := service.Register(ctx, auth.RegisterRequest{
		Email:    email,
		Password: "TestPassword123!",
		FullName: "Juan Gómez",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// El alias debe empezar con "juangomez" (la parte local del email)
	if !strings.HasPrefix(user.Alias, "juangomez") {
		t.Fatalf("alias = %s, esperado empezar con juangomez", user.Alias)
	}
}
func TestRegister_AliasCollision(t *testing.T) {
	service, _ := newTestEnv(t)
	ctx := context.Background()

	ts := time.Now().UnixNano()

	// Mismo prefijo, dominios distintos → colisión de alias base
	email1 := fmt.Sprintf("testcollision%d@test1.local", ts)
	email2 := fmt.Sprintf("testcollision%d@test2.local", ts)

	user1, err := service.Register(ctx, auth.RegisterRequest{
		Email:    email1,
		Password: "TestPassword123!",
		FullName: "User 1",
	})
	if err != nil {
		t.Fatalf("Register 1: %v", err)
	}

	user2, err := service.Register(ctx, auth.RegisterRequest{
		Email:    email2,
		Password: "TestPassword123!",
		FullName: "User 2",
	})
	if err != nil {
		t.Fatalf("Register 2: %v", err)
	}

	if user1.Alias == user2.Alias {
		t.Fatalf("alias duplicado: %s", user1.Alias)
	}
	// El segundo debe tener sufijo numérico
	if user2.Alias != user1.Alias+"2" {
		t.Fatalf("alias = %s, esperado %s2", user2.Alias, user1.Alias)
	}
}
