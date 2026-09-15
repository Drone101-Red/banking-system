package auth

import (
	"context"
	"net/http"
	"strings"

	"banking-system/internal/apperr"
)

// ctxKey es el tipo de las claves que el middleware usa para inyectar
// datos en context.Context.
//
// Es un tipo no exportado (el patrón recomendado por la stdlib de Go)
// para evitar colisiones con claves de otros paquetes.
type ctxKey int

const (
	ctxKeyUserID ctxKey = iota
	ctxKeyTBAccountID
)

// UserIDFromContext devuelve el UserID inyectado por RequireAuth.
//
// Devuelve ("", false) si el middleware no corrió o no había usuario.
func UserIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(ctxKeyUserID).(string)
	return v, ok
}

// TBAccountIDFromContext devuelve el TBAccountID (hex) inyectado por
// RequireAuth. Siempre es un string hex de 32 caracteres.
//
// Devuelve ("", false) si el middleware no corrió o no había usuario.
func TBAccountIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(ctxKeyTBAccountID).(string)
	return v, ok
}

// RequireAuth es el middleware que valida el JWT y, si es válido,
// inyecta UserID y TBAccountID en el context.Context del request.
//
// Validaciones:
//  1. Header Authorization presente con formato "Bearer <token>".
//  2. Token parseable y firmado correctamente (ParseToken).
//  3. sub (UserID) no vacío.
//  4. tb_account_id: 32 caracteres hex (16 bytes).
//
// Si alguna falla, responde 401 con apperr y no llama al siguiente handler.
//
// No consulta PostgreSQL ni TigerBeetle. El JWT contiene toda la
// identidad necesaria. Si el usuario fue desactivado en PG, el token
// sigue siendo válido hasta expirar (24h). Aceptable para el alcance.
func RequireAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := extractBearerToken(r)
			if !ok {
				apperr.Write(w, apperr.Unauthorized(
					"TOKEN_REQUIRED",
					"Se requiere autenticación",
				))
				return
			}

			claims, err := ParseToken(token, secret)
			if err != nil {
				apperr.Write(w, err)
				return
			}

			// Validar forma de los claims antes de inyectarlos.
			if claims.UserID == "" {
				apperr.Write(w, apperr.Unauthorized(
					"TOKEN_INVALID_CLAIMS",
					"Token con claims inválidos",
				))
				return
			}
			if !isValidHex16(claims.TBAccountID) {
				apperr.Write(w, apperr.Unauthorized(
					"TOKEN_INVALID_CLAIMS",
					"Token con claims inválidos",
				))
				return
			}

			ctx := context.WithValue(r.Context(), ctxKeyUserID, claims.UserID)
			ctx = context.WithValue(ctx, ctxKeyTBAccountID, claims.TBAccountID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// extractBearerToken lee el header Authorization y extrae el token.
//
// Formato esperado: "Bearer <token>".
// Devuelve ("", false) si el header no está, está mal formado, o el
// prefijo no es "Bearer " (case-insensitive).
func extractBearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", false
	}
	const prefix = "Bearer "
	if len(h) <= len(prefix) {
		return "", false
	}
	if !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(h[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}

// isValidHex16 verifica que s sea exactamente 32 caracteres hex (16 bytes).
//
// Se usa para validar el claim tb_account_id antes de inyectarlo en el
// context. Un JWT firmado por nosotros con un tb_account_id malformado
// (por bug en GenerateToken, migración de datos, etc.) no debe llegar
// al handler.
func isValidHex16(s string) bool {
	if len(s) != 32 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}
