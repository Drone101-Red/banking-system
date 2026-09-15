// Package auth implementa el registro, login y gestión de sesiones.
package auth

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"

	tb "github.com/tigerbeetle/tigerbeetle-go"

	"banking-system/internal/apperr"
	"banking-system/internal/db"
	"banking-system/internal/models"
	"banking-system/internal/password"
)

// RegisterRequest es el DTO de entrada de POST /api/auth/register.
//
// Las validaciones se aplican en el service (no en el handler) para
// mantener el handler delgado. Ver docs/step3/3.1-3.4-auth-design.md, 3.2.
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
}

// TBAccountCreator abstrae las operaciones que el service necesita de TB.
// Permite mockear en tests unitarios.
type TBAccountCreator interface {
	CreateAccount(id tb.Uint128, code uint16) error
}

// Service orquesta las operaciones de autenticación.
type Service struct {
	pg       *db.PostgresStore
	tbClient TBAccountCreator
}

// NewService construye el Service.
func NewService(pg *db.PostgresStore, tbClient TBAccountCreator) *Service {
	return &Service{
		pg:       pg,
		tbClient: tbClient,
	}
}

// Register crea un usuario nuevo siguiendo el flujo PENDING -> ACTIVE:
//
//  1. Valida y normaliza el request.
//  2. Genera tb_account_id aleatorio.
//  3. Hashea la contraseña con bcrypt.
//  4. INSERT en PostgreSQL con status=PENDING.
//  5. Crea cuenta en TigerBeetle.
//  6. UPDATE en PostgreSQL a status=ACTIVE.
//  7. Devuelve el usuario creado.
//
// Si el paso 5 falla, el usuario queda en PENDING y el reconciliador
// lo recuperará después. No es un error fatal para el cliente más allá
// de devolver 503.
func (s *Service) Register(ctx context.Context, req RegisterRequest) (*models.User, error) {
	// 1. Normalizar
	email := strings.ToLower(strings.TrimSpace(req.Email))
	fullName := strings.TrimSpace(req.FullName)
	// No normalizamos password (ver docs 3.2).

	// 2. Validar
	if err := validateRegister(req, email, fullName); err != nil {
		return nil, err
	}

	// 3. Generar tb_account_id (crypto/rand 16 bytes)
	tbID, tbIDBytes, err := generateTBAccountID()
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("generar tb_account_id: %w", err))
	}

	// 4. Hash password
	hash, err := password.Hash(req.Password)
	if err != nil {
		return nil, apperr.BadRequest("WEAK_PASSWORD", err.Error())
	}

	// 5. INSERT PENDING
	user := &models.User{
		Email:        email,
		PasswordHash: hash,
		FullName:     fullName,
		TBAccountID:  tbIDBytes,
	}
	if err := s.pg.CreateUserPending(ctx, user); err != nil {
		return nil, err
	}

	// 6. Crear cuenta en TB
	if err := s.tbClient.CreateAccount(tbID, models.CodeSavings); err != nil {
		// Usuario queda PENDING. El reconciliador lo recuperará.
		// No borramos el usuario: el diseño dice explícitamente que
		// un PENDING recuperable es preferible a perder el registro.
		return nil, apperr.Internal(fmt.Errorf("crear cuenta TB: %w", err))
	}

	// 7. UPDATE a ACTIVE
	if err := s.pg.UpdateUserStatusActive(ctx, user.ID); err != nil {
		// Mismo razonamiento: el usuario quedará PENDING y el
		// reconciliador lo pasará a ACTIVE en el próximo ciclo.
		return nil, apperr.Internal(fmt.Errorf("activar usuario: %w", err))
	}

	user.Status = models.StatusActive
	return user, nil
}

// validateRegister aplica las validaciones del request.
func validateRegister(req RegisterRequest, email, fullName string) error {
	if email == "" {
		return apperr.BadRequest("EMAIL_REQUIRED", "El email es obligatorio")
	}
	if !strings.Contains(email, "@") || len(email) < 3 {
		return apperr.BadRequest("EMAIL_INVALID", "El email no tiene un formato válido")
	}
	if len(email) > 255 {
		return apperr.BadRequest("EMAIL_TOO_LONG", "El email es demasiado largo")
	}
	if fullName == "" {
		return apperr.BadRequest("FULL_NAME_REQUIRED", "El nombre es obligatorio")
	}
	if len(req.Password) < password.MinLength {
		return apperr.BadRequest("PASSWORD_TOO_SHORT",
			fmt.Sprintf("La contraseña debe tener al menos %d caracteres", password.MinLength))
	}
	if len(req.Password) > password.MaxLength {
		return apperr.BadRequest("PASSWORD_TOO_LONG",
			fmt.Sprintf("La contraseña no puede superar %d bytes", password.MaxLength))
	}
	return nil
}

// generateTBAccountID genera un uint128 aleatorio (crypto/rand) y su
// representación en 16 bytes (big-endian).
//
// Rechaza los IDs 0 y 1 (reservados).
func generateTBAccountID() (tb.Uint128, []byte, error) {
	for {
		var b [16]byte
		if _, err := rand.Read(b[:]); err != nil {
			return tb.Uint128{}, nil, fmt.Errorf("rand.Read: %w", err)
		}
		id := tb.BytesToUint128(b)
		if id == tb.ToUint128(0) || id == tb.ToUint128(1) {
			continue // regenerar
		}
		return id, b[:], nil
	}
}
