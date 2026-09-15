// Package auth implementa el registro, login y gestión de sesiones.
package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"

	"banking-system/internal/apperr"
	"banking-system/internal/db"
	"banking-system/internal/models"
	"banking-system/internal/password"
)

// dummyPasswordHash es un hash bcrypt cost 12 de una contraseña que nadie
// conoce. Se usa para mitigar timing attacks en Login cuando el email no
// existe: comparamos contra este hash para mantener el tiempo de respuesta
// constante, y luego ignoramos el resultado.
//
// Ver docs/step3/3.6-login.md, decisión "timing attack mitigation".
const dummyPasswordHash = "$2a$12$pHpHZ1qj/K0ErwbTaxyDrO2e8KYQRhlJWOWY7EXoKYUq443Wnqfsm"

// RegisterRequest es el DTO de entrada de POST /api/auth/register.
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
}

// LoginRequest es el DTO de entrada de POST /api/auth/login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// TBAccountCreator abstrae las operaciones que el service necesita de TB.
type TBAccountCreator interface {
	CreateAccount(id tb.Uint128, code uint16) error
}

// Service orquesta las operaciones de autenticación.
type Service struct {
	pg        *db.PostgresStore
	tbClient  TBAccountCreator
	jwtSecret string
	jwtExpiry time.Duration
}

// NewService construye el Service.
//
// Requiere el JWT secret y el expiry porque Login los necesita para
// firmar tokens. No hay estado parcialmente inicializado.
func NewService(pg *db.PostgresStore, tbClient TBAccountCreator, jwtSecret string, jwtExpiry time.Duration) *Service {
	return &Service{
		pg:        pg,
		tbClient:  tbClient,
		jwtSecret: jwtSecret,
		jwtExpiry: jwtExpiry,
	}
}

// Register crea un usuario nuevo siguiendo el flujo PENDING -> ACTIVE.
//
// Ver docs/step3/3.5-register.md para el detalle.
func (s *Service) Register(ctx context.Context, req RegisterRequest) (*models.User, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))
	fullName := strings.TrimSpace(req.FullName)

	if err := validateRegister(req, email, fullName); err != nil {
		return nil, err
	}

	tbID, tbIDBytes, err := generateTBAccountID()
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("generar tb_account_id: %w", err))
	}

	hash, err := password.Hash(req.Password)
	if err != nil {
		return nil, apperr.BadRequest("WEAK_PASSWORD", err.Error())
	}

	user := &models.User{
		Email:        email,
		PasswordHash: hash,
		FullName:     fullName,
		TBAccountID:  tbIDBytes,
	}
	if err := s.pg.CreateUserPending(ctx, user); err != nil {
		return nil, err
	}

	if err := s.tbClient.CreateAccount(tbID, models.CodeSavings); err != nil {
		return nil, apperr.Internal(fmt.Errorf("crear cuenta TB: %w", err))
	}

	if err := s.pg.UpdateUserStatusActive(ctx, user.ID); err != nil {
		return nil, apperr.Internal(fmt.Errorf("activar usuario: %w", err))
	}

	user.Status = models.StatusActive
	return user, nil
}

// Login autentica un usuario y devuelve un JWT.
//
// Contrato:
//
//	email vacío                              → 400 EMAIL_REQUIRED
//	password vacía                           → 400 PASSWORD_REQUIRED
//	email inexistente                        → 401 INVALID_CREDENTIALS
//	password incorrecta                      → 401 INVALID_CREDENTIALS
//	password correcta + status PENDING       → 401 ACCOUNT_PENDING
//	password correcta + status ACTIVE        → 200 con user + JWT
//
// Los casos de credenciales inválidas son indistinguibles para el cliente:
// ni el mensaje ni el timing revelan si el email existe.
func (s *Service) Login(ctx context.Context, req LoginRequest) (*models.User, string, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))

	if err := validateLogin(req, email); err != nil {
		return nil, "", err
	}

	// Buscar usuario. Si no existe, mantenemos el timing constante con
	// un bcrypt dummy y devolvemos el mismo error que "password incorrecta".
	user, err := s.pg.GetUserByEmail(ctx, email)
	if err != nil {
		var appErr *apperr.AppError
		if errors.As(err, &appErr) && appErr.Code == "USER_NOT_FOUND" {
			// Timing attack mitigation.
			_ = password.Compare(dummyPasswordHash, req.Password)
			return nil, "", apperr.Unauthorized("INVALID_CREDENTIALS", "Credenciales inválidas")
		}
		return nil, "", err
	}

	// Verificar password.
	if err := password.Compare(user.PasswordHash, req.Password); err != nil {
		if errors.Is(err, password.ErrMismatch) {
			return nil, "", apperr.Unauthorized("INVALID_CREDENTIALS", "Credenciales inválidas")
		}
		return nil, "", apperr.Internal(fmt.Errorf("comparar password: %w", err))
	}

	// Verificar status DESPUÉS de validar password.
	if user.Status == models.StatusPending {
		return nil, "", apperr.Unauthorized("ACCOUNT_PENDING", "Tu cuenta está siendo procesada")
	}
	if user.Status != models.StatusActive {
		return nil, "", apperr.Unauthorized("ACCOUNT_INVALID_STATUS", "Estado de cuenta inválido")
	}

	// Generar JWT.
	token, err := GenerateToken(user, s.jwtSecret, s.jwtExpiry)
	if err != nil {
		return nil, "", apperr.Internal(fmt.Errorf("generar JWT: %w", err))
	}

	return user, token, nil
}

// validateRegister aplica las validaciones del request de registro.
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

// validateLogin aplica las validaciones del request de login.
//
// Deliberadamente laxas: solo verifica que los campos no estén vacíos.
// La validación "dura" (email válido, password correcta) es la del login
// en sí, y se resuelve con un único error genérico.
func validateLogin(req LoginRequest, email string) error {
	if email == "" {
		return apperr.BadRequest("EMAIL_REQUIRED", "El email es obligatorio")
	}
	if req.Password == "" {
		return apperr.BadRequest("PASSWORD_REQUIRED", "La contraseña es obligatoria")
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
			continue
		}
		return id, b[:], nil
	}
}
