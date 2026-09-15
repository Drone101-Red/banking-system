package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	tb "github.com/tigerbeetle/tigerbeetle-go"

	"banking-system/internal/apperr"
	"banking-system/internal/db"
	"banking-system/internal/models"
	"banking-system/internal/password"
)

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

// UserLookup es la respuesta de GET /api/users/lookup.
type UserLookup struct {
	UserID      string `json:"user_id"`
	Email       string `json:"email"`
	FullName    string `json:"full_name"`
	Alias       string `json:"alias"`
	TBAccountID string `json:"tb_account_id"` // hex
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
func NewService(pg *db.PostgresStore, tbClient TBAccountCreator, jwtSecret string, jwtExpiry time.Duration) *Service {
	return &Service{
		pg:        pg,
		tbClient:  tbClient,
		jwtSecret: jwtSecret,
		jwtExpiry: jwtExpiry,
	}
}

// Register crea un usuario nuevo siguiendo el flujo PENDING -> ACTIVE.
func (s *Service) Register(ctx context.Context, req RegisterRequest) (*models.User, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))
	fullName := strings.TrimSpace(req.FullName)

	if err := validateRegister(req, email, fullName); err != nil {
		return nil, err
	}

	alias, err := s.generateAlias(ctx, email)
	if err != nil {
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
		Alias:        alias,
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
func (s *Service) Login(ctx context.Context, req LoginRequest) (*models.User, string, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))

	if err := validateLogin(req, email); err != nil {
		return nil, "", err
	}

	user, err := s.pg.GetUserByEmail(ctx, email)
	if err != nil {
		var appErr *apperr.AppError
		if errors.As(err, &appErr) && appErr.Code == "USER_NOT_FOUND" {
			_ = password.Compare(dummyPasswordHash, req.Password)
			return nil, "", apperr.Unauthorized("INVALID_CREDENTIALS", "Credenciales inválidas")
		}
		return nil, "", err
	}

	if err := password.Compare(user.PasswordHash, req.Password); err != nil {
		if errors.Is(err, password.ErrMismatch) {
			return nil, "", apperr.Unauthorized("INVALID_CREDENTIALS", "Credenciales inválidas")
		}
		return nil, "", apperr.Internal(fmt.Errorf("comparar password: %w", err))
	}

	if user.Status == models.StatusPending {
		return nil, "", apperr.Unauthorized("ACCOUNT_PENDING", "Tu cuenta está siendo procesada")
	}
	if user.Status != models.StatusActive {
		return nil, "", apperr.Unauthorized("ACCOUNT_INVALID_STATUS", "Estado de cuenta inválido")
	}

	token, err := GenerateToken(user, s.jwtSecret, s.jwtExpiry)
	if err != nil {
		return nil, "", apperr.Internal(fmt.Errorf("generar JWT: %w", err))
	}

	return user, token, nil
}

// LookupUser busca un usuario por email o alias.
//
// Exactamente uno de email o alias debe estar presente.
//
// Devuelve solo datos públicos. NO expone password_hash.
func (s *Service) LookupUser(ctx context.Context, email, alias string) (*UserLookup, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	alias = strings.ToLower(strings.TrimSpace(alias))

	if email == "" && alias == "" {
		return nil, apperr.BadRequest("MISSING_QUERY", "Se requiere email o alias")
	}
	if email != "" && alias != "" {
		return nil, apperr.BadRequest("AMBIGUOUS_QUERY", "Solo email o alias, no ambos")
	}

	var (
		user *models.User
		err  error
	)
	if email != "" {
		user, err = s.pg.GetUserByEmail(ctx, email)
	} else {
		user, err = s.pg.GetUserByAlias(ctx, alias)
	}
	if err != nil {
		// Si no existe, devolver error específico de lookup
		var appErr *apperr.AppError
		if errors.As(err, &appErr) && appErr.Code == "USER_NOT_FOUND" {
			return nil, apperr.NotFound("USER_NOT_FOUND", "Usuario no encontrado")
		}
		return nil, err
	}

	// Solo devolver usuarios ACTIVE
	if user.Status != models.StatusActive {
		return nil, apperr.NotFound("USER_NOT_FOUND", "Usuario no encontrado")
	}

	return &UserLookup{
		UserID:      user.ID,
		Email:       user.Email,
		FullName:    user.FullName,
		Alias:       user.Alias,
		TBAccountID: hexEncode(user.TBAccountID),
	}, nil
}

// generateAlias genera un alias único basado en el email.
func (s *Service) generateAlias(ctx context.Context, email string) (string, error) {
	local := email
	if idx := strings.Index(email, "@"); idx > 0 {
		local = email[:idx]
	}

	var cleaned strings.Builder
	for _, r := range local {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '.' {
			cleaned.WriteRune(unicode.ToLower(r))
		}
	}
	base := cleaned.String()

	if len(base) < 3 {
		base = fmt.Sprintf("user-%d", time.Now().UnixNano()%10000)
	}
	if len(base) > 20 {
		base = base[:20]
	}

	exists, err := s.pg.AliasExists(ctx, base)
	if err != nil {
		return "", err
	}
	if !exists {
		return base, nil
	}

	for i := 2; i <= 100; i++ {
		candidate := fmt.Sprintf("%s%d", base, i)
		exists, err := s.pg.AliasExists(ctx, candidate)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}

	return fmt.Sprintf("user-%d", time.Now().UnixNano()%100000), nil
}

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

func validateLogin(req LoginRequest, email string) error {
	if email == "" {
		return apperr.BadRequest("EMAIL_REQUIRED", "El email es obligatorio")
	}
	if req.Password == "" {
		return apperr.BadRequest("PASSWORD_REQUIRED", "La contraseña es obligatoria")
	}
	return nil
}

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

// hexEncode convierte bytes a hex string.
func hexEncode(b []byte) string {
	const hexChars = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hexChars[v>>4]
		out[i*2+1] = hexChars[v&0x0f]
	}
	return string(out)
}
