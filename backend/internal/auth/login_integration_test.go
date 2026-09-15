//go:build integration

package auth_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"

	"banking-system/internal/auth"
	"banking-system/internal/models"
	"banking-system/internal/password"
)

// randomTBAccountIDBytes genera 16 bytes aleatorios para un tb_account_id
// de prueba. No crea la cuenta en TigerBeetle: eso es exactamente lo que
// queremos simular para el test de usuario PENDING.
func randomTBAccountIDBytes(t *testing.T) []byte {
	t.Helper()

	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	// Asegurar que no sea 0 (reservado)
	allZero := true
	for _, x := range b {
		if x != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		b[15] = 1
	}
	return b[:]
}

// registerAndGetUser registra un usuario y devuelve el email y password
// para reusar en tests de login.
func registerAndGetUser(t *testing.T, service *auth.Service) (string, string) {
	t.Helper()
	ctx := context.Background()

	email := uniqueEmail("login")
	pass := "TestPassword123!"

	_, err := service.Register(ctx, auth.RegisterRequest{
		Email:    email,
		Password: pass,
		FullName: "Login Test",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	return email, pass
}

// --- Login ---

func TestLogin_HappyPath(t *testing.T) {
	service, _ := newTestEnv(t)
	ctx := context.Background()

	email, pass := registerAndGetUser(t, service)

	user, token, err := service.Login(ctx, auth.LoginRequest{
		Email:    email,
		Password: pass,
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if user == nil {
		t.Fatal("user nil")
	}
	if user.Email != email {
		t.Fatalf("email = %s, esperado %s", user.Email, email)
	}
	if user.Status != models.StatusActive {
		t.Fatalf("status = %s, esperado ACTIVE", user.Status)
	}
	if token == "" {
		t.Fatal("token vacío")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	service, _ := newTestEnv(t)
	ctx := context.Background()

	email, _ := registerAndGetUser(t, service)

	_, _, err := service.Login(ctx, auth.LoginRequest{
		Email:    email,
		Password: "WrongPassword!",
	})
	if err == nil {
		t.Fatal("esperaba error por password incorrecta")
	}
	if !strings.Contains(err.Error(), "INVALID_CREDENTIALS") {
		t.Fatalf("código esperado INVALID_CREDENTIALS, obtuve: %v", err)
	}
}

func TestLogin_EmailNotFound(t *testing.T) {
	service, _ := newTestEnv(t)
	ctx := context.Background()

	_, _, err := service.Login(ctx, auth.LoginRequest{
		Email:    "noexiste-" + uniqueEmail("nf"),
		Password: "AnyPassword123!",
	})
	if err == nil {
		t.Fatal("esperaba error por email inexistente")
	}
	if !strings.Contains(err.Error(), "INVALID_CREDENTIALS") {
		t.Fatalf("código esperado INVALID_CREDENTIALS, obtuve: %v", err)
	}
}

func TestLogin_PendingUser(t *testing.T) {
	service, pg := newTestEnv(t)
	ctx := context.Background()

	// Crear usuario PENDING directamente con CreateUserPending.
	// No creamos cuenta en TigerBeetle: el escenario PENDING real es
	// exactamente este (usuario en PG, cuenta TB ausente).
	email := uniqueEmail("pending")
	pass := "TestPassword123!"

	hash, err := password.Hash(pass)
	if err != nil {
		t.Fatalf("password.Hash: %v", err)
	}

	user := &models.User{
		Email:        email,
		PasswordHash: hash,
		FullName:     "Pending User",
		TBAccountID:  randomTBAccountIDBytes(t),
	}
	if err := pg.CreateUserPending(ctx, user); err != nil {
		t.Fatalf("CreateUserPending: %v", err)
	}

	// Login debe devolver ACCOUNT_PENDING, NO INVALID_CREDENTIALS.
	// La password es correcta, pero el status impide el login.
	_, _, err = service.Login(ctx, auth.LoginRequest{
		Email:    email,
		Password: pass,
	})
	if err == nil {
		t.Fatal("esperaba error por usuario PENDING")
	}
	if !strings.Contains(err.Error(), "ACCOUNT_PENDING") {
		t.Fatalf("código esperado ACCOUNT_PENDING, obtuve: %v", err)
	}
}

func TestLogin_Validation_EmptyEmail(t *testing.T) {
	service, _ := newTestEnv(t)
	ctx := context.Background()

	_, _, err := service.Login(ctx, auth.LoginRequest{
		Email:    "",
		Password: "AnyPassword123!",
	})
	if err == nil {
		t.Fatal("esperaba error por email vacío")
	}
	if !strings.Contains(err.Error(), "EMAIL_REQUIRED") {
		t.Fatalf("código esperado EMAIL_REQUIRED, obtuve: %v", err)
	}
}

func TestLogin_Validation_EmptyPassword(t *testing.T) {
	service, _ := newTestEnv(t)
	ctx := context.Background()

	_, _, err := service.Login(ctx, auth.LoginRequest{
		Email:    "algo@example.com",
		Password: "",
	})
	if err == nil {
		t.Fatal("esperaba error por password vacía")
	}
	if !strings.Contains(err.Error(), "PASSWORD_REQUIRED") {
		t.Fatalf("código esperado PASSWORD_REQUIRED, obtuve: %v", err)
	}
}

func TestLogin_TokenIsValid(t *testing.T) {
	service, _ := newTestEnv(t)
	ctx := context.Background()

	email, pass := registerAndGetUser(t, service)

	_, token, err := service.Login(ctx, auth.LoginRequest{
		Email:    email,
		Password: pass,
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	// Parsear el token con el mismo secret del newTestEnv.
	claims, err := auth.ParseToken(token, testJWTSecret)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.UserID == "" {
		t.Fatal("claims.UserID vacío")
	}
	if claims.TBAccountID == "" {
		t.Fatal("claims.TBAccountID vacío")
	}
}

func TestLogin_TokenHasCorrectClaims(t *testing.T) {
	service, pg := newTestEnv(t)
	ctx := context.Background()

	email, pass := registerAndGetUser(t, service)

	user, token, err := service.Login(ctx, auth.LoginRequest{
		Email:    email,
		Password: pass,
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	claims, err := auth.ParseToken(token, testJWTSecret)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}

	if claims.UserID != user.ID {
		t.Fatalf("claims.UserID = %s, esperado %s", claims.UserID, user.ID)
	}

	// Verificar que el tb_account_id del claim coincide con el de PG.
	fetched, err := pg.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}

	expectedHex := hex.EncodeToString(fetched.TBAccountID)
	if claims.TBAccountID != expectedHex {
		t.Fatalf("claims.TBAccountID = %s, esperado %s", claims.TBAccountID, expectedHex)
	}
}
