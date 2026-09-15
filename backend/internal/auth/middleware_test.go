package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"banking-system/internal/models"
)

const testSecretMW = "test-secret-at-least-32-bytes-long-ok"

func makeMWTestUser() *models.User {
	return &models.User{
		ID:          "550e8400-e29b-41d4-a716-446655440000",
		Email:       "juan@example.com",
		FullName:    "Juan Pérez",
		TBAccountID: []byte{0xc4, 0x42, 0x93, 0x1d, 0xf9, 0xfc, 0xfd, 0x81, 0xbc, 0x6f, 0x7e, 0x3e, 0x3b, 0x2e, 0xcb, 0xb1},
		Status:      models.StatusActive,
	}
}

// handlerProbe captura los valores inyectados en el context.
func handlerProbe(t *testing.T, wantUserID, wantTBID string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := UserIDFromContext(r.Context())
		if !ok {
			t.Errorf("UserIDFromContext: !ok")
		}
		if userID != wantUserID {
			t.Errorf("UserID = %s, esperado %s", userID, wantUserID)
		}
		tbID, ok := TBAccountIDFromContext(r.Context())
		if !ok {
			t.Errorf("TBAccountIDFromContext: !ok")
		}
		if tbID != wantTBID {
			t.Errorf("TBAccountID = %s, esperado %s", tbID, wantTBID)
		}
		w.WriteHeader(http.StatusOK)
	})
}

func TestRequireAuth_Valid(t *testing.T) {
	user := makeMWTestUser()
	token, err := GenerateToken(user, testSecretMW, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	mw := RequireAuth(testSecretMW)
	handler := mw(handlerProbe(t, user.ID, "c442931df9fcfd81bc6f7e3e3b2ecbb1"))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200. Body: %s", rec.Code, rec.Body.String())
	}
}

func TestRequireAuth_MissingHeader(t *testing.T) {
	mw := RequireAuth(testSecretMW)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler no debería llamarse")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado 401", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "TOKEN_REQUIRED") {
		t.Fatalf("código esperado TOKEN_REQUIRED, obtuve: %s", rec.Body.String())
	}
}

func TestRequireAuth_WrongPrefix(t *testing.T) {
	mw := RequireAuth(testSecretMW)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler no debería llamarse")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Token abc.def.ghi")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado 401", rec.Code)
	}
}

func TestRequireAuth_InvalidToken(t *testing.T) {
	mw := RequireAuth(testSecretMW)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler no debería llamarse")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer no.es.un.jwt")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado 401", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "TOKEN_INVALID") {
		t.Fatalf("código esperado TOKEN_INVALID, obtuve: %s", rec.Body.String())
	}
}

func TestRequireAuth_ExpiredToken(t *testing.T) {
	user := makeMWTestUser()
	token, err := GenerateToken(user, testSecretMW, -time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	mw := RequireAuth(testSecretMW)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler no debería llamarse")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado 401", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "TOKEN_EXPIRED") {
		t.Fatalf("código esperado TOKEN_EXPIRED, obtuve: %s", rec.Body.String())
	}
}

func TestRequireAuth_WrongSecret(t *testing.T) {
	user := makeMWTestUser()
	token, err := GenerateToken(user, "otro-secret-32-bytes-long-aaaaaaa", time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	mw := RequireAuth(testSecretMW)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler no debería llamarse")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado 401", rec.Code)
	}
}

func TestRequireAuth_InvalidTBAccountIDClaim(t *testing.T) {
	// Construir un token válido con un tb_account_id malformado.
	// Esto no debería pasar en producción (GenerateToken lo valida),
	// pero el middleware debe defenderse.
	claims := Claims{
		UserID:           "550e8400-e29b-41d4-a716-446655440000",
		TBAccountID:      "no-es-hex-valido", // ← malformado
		RegisteredClaims: jwtRegisteredClaims(time.Now(), time.Hour),
	}
	token := signTestClaims(t, claims, testSecretMW)

	mw := RequireAuth(testSecretMW)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler no debería llamarse con claim malformado")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado 401", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "TOKEN_INVALID_CLAIMS") {
		t.Fatalf("código esperado TOKEN_INVALID_CLAIMS, obtuve: %s", rec.Body.String())
	}
}

func TestRequireAuth_EmptyUserIDClaim(t *testing.T) {
	claims := Claims{
		UserID:           "", // ← vacío
		TBAccountID:      "c442931df9fcfd81bc6f7e3e3b2ecbb1",
		RegisteredClaims: jwtRegisteredClaims(time.Now(), time.Hour),
	}
	token := signTestClaims(t, claims, testSecretMW)

	mw := RequireAuth(testSecretMW)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler no debería llamarse con claim vacío")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado 401", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "TOKEN_INVALID_CLAIMS") {
		t.Fatalf("código esperado TOKEN_INVALID_CLAIMS, obtuve: %s", rec.Body.String())
	}
}

// --- Helpers de test ---

func TestExtractBearerToken_CaseInsensitive(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "bearer abc.def.ghi")
	token, ok := extractBearerToken(req)
	if !ok || token != "abc.def.ghi" {
		t.Fatalf("esperaba token extraído, obtuve ok=%v token=%q", ok, token)
	}
}

func TestIsValidHex16(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"c442931df9fcfd81bc6f7e3e3b2ecbb1", true},
		{"C442931DF9FCFD81BC6F7E3E3B2ECBB1", true},
		{"", false},
		{"corto", false},
		{"c442931df9fcfd81bc6f7e3e3b2ecbb1extra", false},
		{"g442931df9fcfd81bc6f7e3e3b2ecbb1", false}, // 'g' no es hex
		{"c442931df9fcfd81bc6f7e3e3b2ecbb-", false},
	}
	for _, c := range cases {
		if got := isValidHex16(c.in); got != c.want {
			t.Errorf("isValidHex16(%q) = %v, esperado %v", c.in, got, c.want)
		}
	}
}
func jwtRegisteredClaims(now time.Time, expiry time.Duration) jwt.RegisteredClaims {
	return jwt.RegisteredClaims{
		Issuer:    Issuer,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
	}
}

func signTestClaims(t *testing.T, claims Claims, secret string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}
	return signed
}
