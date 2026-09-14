// Package models contiene los tipos y constantes compartidos del dominio.
package models

import (
	"time"
)

// UserStatus representa el estado del ciclo de vida de un usuario.
type UserStatus string

const (
	// StatusPending indica que el usuario existe en PostgreSQL pero su
	// cuenta en TigerBeetle aún no está confirmada como vinculada.
	// Un usuario PENDING no puede iniciar sesión.
	StatusPending UserStatus = "PENDING"

	// StatusActive indica que el usuario tiene una identidad PostgreSQL
	// válida y una cuenta TigerBeetle confirmada.
	// Solo un usuario ACTIVE puede iniciar sesión.
	StatusActive UserStatus = "ACTIVE"
)

// User representa un usuario en PostgreSQL.
//
// Relación con TigerBeetle:
//   - ID (UUID)         → identidad de aplicación (auth, JWT, relaciones).
//   - TBAccountID       → identidad financiera (uint128 en TigerBeetle).
//
// El campo TBAccountID se almacena como []byte (16 bytes) porque PostgreSQL
// usa BYTEA para uint128.
type User struct {
	ID           string     `json:"id"`
	Email        string     `json:"email"`
	PasswordHash string     `json:"-"` // nunca se serializa a JSON
	FullName     string     `json:"full_name"`
	TBAccountID  []byte     `json:"-"` // uint128 como 16 bytes
	Status       UserStatus `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}
