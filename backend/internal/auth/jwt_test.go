package auth

import (
	"testing"
	"time"

	"banking-system/internal/models"

	tbjwt "github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret-at-least-32-bytes-long-ok"

func makeTestUser() *models.User {
	return &models.User{
		ID:          "550e8400-e29b-41d4-a716-446655440000",
		Email:       "juan@example.com",
		FullName:    "Juan Pérez",
		TBAccountID: []byte{0xc4, 0x42, 0x93, 0x1d, 0xf9, 0xfc, 0xfd, 0x81, 0xbc, 0x6f, 0x7e, 0x3e, 0x3b, 0x2e, 0xcb, 0xb1},
		Status:      models.StatusActive,
	}
}

func TestGenerateToken_Valid(t *testing.T) {
	user := makeTestUser()
	token, err := GenerateToken(user, testSecret, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if token == "" {
		t.Fatal("token vacío")
	}
}

func TestGenerateToken_NilUser(t *testing.T) {
	_, err := GenerateToken(nil, testSecret, time.Hour)
	if err == nil {
		t.Fatal("esperaba error por user nil")
	}
}

func TestGenerateToken_EmptyID(t *testing.T) {
	user := makeTestUser()
	user.ID = ""
	_, err := GenerateToken(user, testSecret, time.Hour)
	if err == nil {
		t.Fatal("esperaba error por ID vacío")
	}
}

func TestGenerateToken_WrongTBIDLength(t *testing.T) {
	user := makeTestUser()
	user.TBAccountID = []byte{0x01, 0x02} // solo 2 bytes
	_, err := GenerateToken(user, testSecret, time.Hour)
	if err == nil {
		t.Fatal("esperaba error por TBAccountID mal formado")
	}
}

func TestParseToken_Valid(t *testing.T) {
	user := makeTestUser()
	token, err := GenerateToken(user, testSecret, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	claims, err := ParseToken(token, testSecret)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.UserID != user.ID {
		t.Fatalf("UserID = %s, esperado %s", claims.UserID, user.ID)
	}
	if claims.TBAccountID != "c442931df9fcfd81bc6f7e3e3b2ecbb1" {
		t.Fatalf("TBAccountID = %s, esperado c442931df9fcfd81bc6f7e3e3b2ecbb1", claims.TBAccountID)
	}
	if claims.Issuer != Issuer {
		t.Fatalf("Issuer = %s, esperado %s", claims.Issuer, Issuer)
	}
}

func TestParseToken_WrongSecret(t *testing.T) {
	user := makeTestUser()
	token, err := GenerateToken(user, testSecret, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	_, err = ParseToken(token, "different-secret-32-bytes-long-aaaaa")
	if err == nil {
		t.Fatal("esperaba error por secret distinto")
	}
}

func TestParseToken_Expired(t *testing.T) {
	user := makeTestUser()
	// Token con expiración en el pasado
	token, err := GenerateToken(user, testSecret, -time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	_, err = ParseToken(token, testSecret)
	if err == nil {
		t.Fatal("esperaba error por token expirado")
	}
}

func TestParseToken_Malformed(t *testing.T) {
	_, err := ParseToken("esto.no.es.un.jwt", testSecret)
	if err == nil {
		t.Fatal("esperaba error por token malformado")
	}
}

func TestParseToken_WrongIssuer(t *testing.T) {
	user := makeTestUser()

	// Firmar con un Issuer distinto al esperado
	claims := Claims{
		UserID:      user.ID,
		TBAccountID: "c442931df9fcfd81bc6f7e3e3b2ecbb1",
		RegisteredClaims: tbjwt.RegisteredClaims{
			Issuer:    "otro-issuer",
			IssuedAt:  tbjwt.NewNumericDate(time.Now()),
			ExpiresAt: tbjwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := tbjwt.NewWithClaims(tbjwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}

	_, err = ParseToken(signed, testSecret)
	if err == nil {
		t.Fatal("esperaba error por issuer distinto")
	}
}

func TestParseToken_AlgNone(t *testing.T) {
	// Construir un token con alg=none para verificar que se rechaza.
	claims := Claims{
		UserID:      "atacante",
		TBAccountID: "00000000000000000000000000000000",
		RegisteredClaims: tbjwt.RegisteredClaims{
			Issuer:    Issuer,
			IssuedAt:  tbjwt.NewNumericDate(time.Now()),
			ExpiresAt: tbjwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := tbjwt.NewWithClaims(tbjwt.SigningMethodNone, claims)
	signed, err := token.SignedString(tbjwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}

	_, err = ParseToken(signed, testSecret)
	if err == nil {
		t.Fatal("esperaba error por alg=none")
	}
}
