package chat

import (
	"context"
	"encoding/json"
	"time"

	"banking-system/internal/apperr"
	"banking-system/internal/db"
)

// ConfirmationTTL es el tiempo máximo que una operación pendiente
// puede esperar confirmación antes de expirar.
const ConfirmationTTL = 5 * time.Minute

// PendingOperation representa una operación de escritura que espera
// confirmación del usuario.
type PendingOperation struct {
	Token       string    `json:"confirmation_token"`
	Type        string    `json:"type"`
	AmountCents int64     `json:"amount_cents"`
	ToAccountID string    `json:"to_account_id,omitempty"`
	ExpiresAt   time.Time `json:"expires_at"`

	// UserID no se expone al cliente.
	UserID string `json:"-"`
}

// ConfirmationStore abstrae el storage de operaciones pendientes.
type ConfirmationStore interface {
	Save(ctx context.Context, op *PendingOperation) error
	Consume(ctx context.Context, token, userID string) (*PendingOperation, error)
}

// PostgresConfirmationStore implementa ConfirmationStore con PostgreSQL.
type PostgresConfirmationStore struct {
	pg *db.PostgresStore
}

// NewPostgresConfirmationStore construye el store.
func NewPostgresConfirmationStore(pg *db.PostgresStore) *PostgresConfirmationStore {
	return &PostgresConfirmationStore{pg: pg}
}

// Save persiste una operación pendiente y le asigna un token.
func (s *PostgresConfirmationStore) Save(ctx context.Context, op *PendingOperation) error {
	operationJSON, err := json.Marshal(map[string]any{
		"type":          op.Type,
		"amount_cents":  op.AmountCents,
		"to_account_id": op.ToAccountID,
	})
	if err != nil {
		return apperr.Internal(err)
	}

	token, expiresAt, err := s.pg.SavePendingConfirmation(
		ctx,
		op.UserID,
		operationJSON,
		time.Now().UTC().Add(ConfirmationTTL),
	)
	if err != nil {
		return err
	}
	op.Token = token
	op.ExpiresAt = expiresAt
	return nil
}

// Consume recupera y marca como usada una operación pendiente.
func (s *PostgresConfirmationStore) Consume(
	ctx context.Context,
	token, userID string,
) (*PendingOperation, error) {
	operationJSON, err := s.pg.ConsumePendingConfirmation(ctx, token, userID)
	if err != nil {
		return nil, err
	}

	var opData struct {
		Type        string `json:"type"`
		AmountCents int64  `json:"amount_cents"`
		ToAccountID string `json:"to_account_id"`
	}
	if err := json.Unmarshal(operationJSON, &opData); err != nil {
		return nil, apperr.Internal(err)
	}

	return &PendingOperation{
		Token:       token,
		Type:        opData.Type,
		AmountCents: opData.AmountCents,
		ToAccountID: opData.ToAccountID,
		UserID:      userID,
	}, nil
}
