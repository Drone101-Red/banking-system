package auth

import (
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"banking-system/internal/apperr"
	"banking-system/internal/models"
)

// Issuer identifica quién emite los tokens.
const Issuer = "banking-system"

// Errores de JWT.
var (
	ErrInvalidToken = errors.New("token inválido")
	ErrExpiredToken = errors.New("token expirado")
)

// Claims es la estructura de claims del JWT.
//
// Incluye únicamente identidad estable:
//   - UserID:      UUID del usuario (app).
//   - TBAccountID: uint128 en hex (financiero).
//
// NO incluye información mutable (saldo, status) porque el JWT es
// una fotografía del momento de emisión. Ver docs/step3/3.6-login.md.
type Claims struct {
	UserID      string `json:"sub"`
	TBAccountID string `json:"tb_account_id"`
	jwt.RegisteredClaims
}

// GenerateToken firma un JWT para el usuario.
//
// El token incluye:
//   - sub:            user.ID
//   - tb_account_id:  hex del TBAccountID
//   - iss:            Issuer
//   - iat:            ahora
//   - exp:            ahora + expiry
func GenerateToken(user *models.User, secret string, expiry time.Duration) (string, error) {
	if user == nil {
		return "", fmt.Errorf("usuario nil")
	}
	if user.ID == "" {
		return "", fmt.Errorf("user.ID vacío")
	}
	if len(user.TBAccountID) != 16 {
		return "", fmt.Errorf("TBAccountID debe ser 16 bytes, es %d", len(user.TBAccountID))
	}

	now := time.Now()
	claims := Claims{
		UserID:      user.ID,
		TBAccountID: hex.EncodeToString(user.TBAccountID),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    Issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("firmar JWT: %w", err)
	}
	return signed, nil
}

// ParseToken valida y decodifica un JWT.
//
// Verifica:
//   - algoritmo HS256 (evita "alg: none" attacks)
//   - firma con el secret
//   - expiración
//
// Devuelve apperr.Unauthorized si el token es inválido o expirado.
func ParseToken(tokenString, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(
		tokenString,
		&Claims{},
		func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("método de firma inesperado: %v", t.Header["alg"])
			}
			return []byte(secret), nil
		},
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithIssuer(Issuer),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, apperr.Unauthorized("TOKEN_EXPIRED", "El token ha expirado")
		}
		return nil, apperr.Unauthorized("TOKEN_INVALID", "Token inválido")
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, apperr.Unauthorized("TOKEN_INVALID", "Token inválido")
	}

	return claims, nil
}
