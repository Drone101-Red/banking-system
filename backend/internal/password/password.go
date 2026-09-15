// Package password encapsula el hashing y verificación de contraseñas.
//
// Usa bcrypt con cost 12 (ver docs/step3/3.1-3.4-auth-design.md, decisión 3.4).
package password

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

const (
	// BcryptCost es el coste usado para generar hashes.
	// 12 implica ~4x el trabajo de 10, encareciendo ataques offline.
	BcryptCost = 12

	// MinLength es la longitud mínima aceptada en bytes.
	MinLength = 8

	// MaxLength es la longitud máxima aceptada en bytes.
	//
	// bcrypt solo usa los primeros 72 bytes del input. Para evitar
	// comportamientos silenciosos, rechazamos contraseñas más largas.
	MaxLength = 72
)

// Errores del paquete.
var (
	ErrTooShort = errors.New("password demasiado corta")
	ErrTooLong  = errors.New("password demasiado larga")
	ErrMismatch = errors.New("password no coincide")
)

// Hash genera un hash bcrypt de la contraseña.
//
// Valida longitud antes de hashear. No trunca silenciosamente.
func Hash(plain string) (string, error) {
	if len(plain) < MinLength {
		return "", fmt.Errorf("%w: mínimo %d bytes", ErrTooShort, MinLength)
	}
	if len(plain) > MaxLength {
		return "", fmt.Errorf("%w: máximo %d bytes", ErrTooLong, MaxLength)
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(plain), BcryptCost)
	if err != nil {
		return "", fmt.Errorf("bcrypt hash: %w", err)
	}
	return string(hashed), nil
}

// Compare verifica que una contraseña coincide con un hash.
//
// Devuelve nil si coincide, ErrMismatch si no.
func Compare(hash, plain string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return ErrMismatch
	}
	if err != nil {
		return fmt.Errorf("bcrypt compare: %w", err)
	}
	return nil
}
