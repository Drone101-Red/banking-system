// Package models contiene los tipos y constantes compartidos del dominio.
package models

import (
	"time"
)

// UserStatus representa el estado del ciclo de vida de un usuario.
type UserStatus string

const (
	StatusPending UserStatus = "PENDING"
	StatusActive  UserStatus = "ACTIVE"
)

// User representa un usuario en PostgreSQL.
type User struct {
	ID           string     `json:"id"`
	Email        string     `json:"email"`
	PasswordHash string     `json:"-"`
	FullName     string     `json:"full_name"`
	Alias        string     `json:"alias"`
	TBAccountID  []byte     `json:"-"`
	Status       UserStatus `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}
