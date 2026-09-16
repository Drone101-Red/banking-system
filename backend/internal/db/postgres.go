package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"banking-system/internal/apperr"
	"banking-system/internal/models"
)

const (
	pgMaxOpenConns = 20
	pgMaxIdleConns = 5

	pgConnectionAttempts   = 10
	pgConnectionRetryDelay = 1 * time.Second
)

const pgUniqueViolation = "23505"

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(dsn string) (*PostgresStore, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("abrir postgres: %w", err)
	}
	db.SetMaxOpenConns(pgMaxOpenConns)
	db.SetMaxIdleConns(pgMaxIdleConns)

	var lastErr error
	for attempt := 1; attempt <= pgConnectionAttempts; attempt++ {
		if err := db.Ping(); err == nil {
			return &PostgresStore{db: db}, nil
		} else {
			lastErr = err
		}
		if attempt < pgConnectionAttempts {
			time.Sleep(pgConnectionRetryDelay)
		}
	}

	_ = db.Close()
	return nil, fmt.Errorf(
		"postgres no disponible después de %d intentos: %w",
		pgConnectionAttempts, lastErr,
	)
}

func (s *PostgresStore) Close() error { return s.db.Close() }

func (s *PostgresStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// CreateUserPending inserta un usuario nuevo con status PENDING.
func (s *PostgresStore) CreateUserPending(ctx context.Context, u *models.User) error {
	const q = `
		INSERT INTO users (email, password_hash, full_name, alias, tb_account_id, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at`

	err := s.db.QueryRowContext(ctx, q,
		u.Email,
		u.PasswordHash,
		u.FullName,
		u.Alias,
		u.TBAccountID,
		models.StatusPending,
	).Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)

	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && string(pqErr.Code) == pgUniqueViolation {
			switch pqErr.Constraint {
			case "users_email_key":
				return apperr.Conflict("EMAIL_EXISTS", "El email ya está registrado")
			case "users_alias_key":
				return apperr.Conflict("ALIAS_EXISTS", "El alias ya está en uso")
			case "users_tb_account_id_key":
				return apperr.Conflict("TB_ACCOUNT_ID_COLLISION", "Colisión de identificador financiero")
			default:
				return apperr.Conflict("UNIQUE_VIOLATION", "Recurso duplicado")
			}
		}
		return apperr.Internal(fmt.Errorf("create user pending: %w", err))
	}

	u.Status = models.StatusPending
	return nil
}

// GetUserByEmail busca un usuario por email.
func (s *PostgresStore) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	const q = `
		SELECT id, email, password_hash, full_name, alias, tb_account_id,
		       status, created_at, updated_at
		FROM users
		WHERE email = $1`

	return s.scanUser(s.db.QueryRowContext(ctx, q, email))
}

// GetUserByID busca un usuario por UUID.
func (s *PostgresStore) GetUserByID(ctx context.Context, id string) (*models.User, error) {
	const q = `
		SELECT id, email, password_hash, full_name, alias, tb_account_id,
		       status, created_at, updated_at
		FROM users
		WHERE id = $1`

	return s.scanUser(s.db.QueryRowContext(ctx, q, id))
}

// GetUserByAlias busca un usuario por alias.
func (s *PostgresStore) GetUserByAlias(ctx context.Context, alias string) (*models.User, error) {
	const q = `
		SELECT id, email, password_hash, full_name, alias, tb_account_id,
		       status, created_at, updated_at
		FROM users
		WHERE alias = $1`

	return s.scanUser(s.db.QueryRowContext(ctx, q, alias))
}

// GetUserByTBAccountID busca un usuario por su tb_account_id.
func (s *PostgresStore) GetUserByTBAccountID(ctx context.Context, tbID []byte) (*models.User, error) {
	const q = `
		SELECT id, email, password_hash, full_name, alias, tb_account_id,
		       status, created_at, updated_at
		FROM users
		WHERE tb_account_id = $1`

	return s.scanUser(s.db.QueryRowContext(ctx, q, tbID))
}

// UpdateUserStatusActive marca un usuario PENDING como ACTIVE.
func (s *PostgresStore) UpdateUserStatusActive(ctx context.Context, id string) error {
	const q = `
		UPDATE users
		SET status = $1,
		    updated_at = NOW()
		WHERE id = $2 AND status = $3`

	res, err := s.db.ExecContext(ctx, q, models.StatusActive, id, models.StatusPending)
	if err != nil {
		return apperr.Internal(fmt.Errorf("update user status active: %w", err))
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return apperr.Internal(fmt.Errorf("rows affected: %w", err))
	}

	if rows == 0 {
		exists, err := s.userExists(ctx, id)
		if err != nil {
			return err
		}
		if !exists {
			return apperr.NotFound("USER_NOT_FOUND", "Usuario no encontrado")
		}
	}

	return nil
}

// ListPendingUsers devuelve hasta `limit` usuarios con status PENDING.
func (s *PostgresStore) ListPendingUsers(ctx context.Context, limit int) ([]*models.User, error) {
	const q = `
		SELECT id, email, password_hash, full_name, alias, tb_account_id,
		       status, created_at, updated_at
		FROM users
		WHERE status = $1
		ORDER BY created_at ASC
		LIMIT $2`

	rows, err := s.db.QueryContext(ctx, q, models.StatusPending, limit)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("list pending users: %w", err))
	}
	defer rows.Close()

	var out []*models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(
			&u.ID, &u.Email, &u.PasswordHash, &u.FullName, &u.Alias,
			&u.TBAccountID, &u.Status, &u.CreatedAt, &u.UpdatedAt,
		); err != nil {
			return nil, apperr.Internal(fmt.Errorf("scan pending user: %w", err))
		}
		out = append(out, &u)
	}
	if err := rows.Err(); err != nil {
		return nil, apperr.Internal(fmt.Errorf("rows error: %w", err))
	}
	return out, nil
}

// AliasExists devuelve true si el alias ya está tomado.
func (s *PostgresStore) AliasExists(ctx context.Context, alias string) (bool, error) {
	const q = `SELECT 1 FROM users WHERE alias = $1`
	var dummy int
	err := s.db.QueryRowContext(ctx, q, alias).Scan(&dummy)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, apperr.Internal(fmt.Errorf("alias exists: %w", err))
	}
	return true, nil
}

// --- Transactions log ---
// InsertTransactionLog guarda un índice de la transferencia para el
// historial. La verdad financiera está en TigerBeetle.
//
// Es idempotente: si el tb_transfer_id ya existe (UNIQUE constraint),
// el INSERT usa ON CONFLICT DO NOTHING.
//
// Si t.FixtureCreatedAt está seteado, se usa como created_at.
// Si no, se usa NOW().
func (s *PostgresStore) InsertTransactionLog(ctx context.Context, t *models.Transaction) error {
	const q = `
		INSERT INTO transactions_log
		    (tb_transfer_id, debit_account_id, credit_account_id, amount_cents, code, description, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, COALESCE($7, NOW()))
		ON CONFLICT (tb_transfer_id) DO NOTHING
		RETURNING id, created_at`

	err := s.db.QueryRowContext(ctx, q,
		t.TBTransferID,
		t.DebitAccountID,
		t.CreditAccountID,
		t.AmountCents,
		t.Code,
		t.Description,
		t.FixtureCreatedAt,
	).Scan(&t.ID, &t.CreatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return apperr.Internal(fmt.Errorf("insert transaction log: %w", err))
	}
	return nil
}

// UpdateTransactionLogMetadata actualiza description y created_at de una
// fila en transactions_log.
func (s *PostgresStore) UpdateTransactionLogMetadata(
	ctx context.Context,
	tbTransferID []byte,
	description string,
	createdAt time.Time,
) error {
	const q = `
		UPDATE transactions_log
		SET description = $1, created_at = $2
		WHERE tb_transfer_id = $3`
	_, err := s.db.ExecContext(ctx, q, description, createdAt, tbTransferID)
	if err != nil {
		return apperr.Internal(fmt.Errorf("update txlog metadata: %w", err))
	}
	return nil
}

// ListTransactions devuelve el historial paginado.
func (s *PostgresStore) ListTransactions(
	ctx context.Context,
	tbAccountID []byte,
	limit, offset int,
) ([]*models.Transaction, int, error) {
	const countQ = `
		SELECT COUNT(*) FROM transactions_log
		WHERE debit_account_id = $1 OR credit_account_id = $1`

	var total int
	if err := s.db.QueryRowContext(ctx, countQ, tbAccountID).Scan(&total); err != nil {
		return nil, 0, apperr.Internal(fmt.Errorf("count transactions: %w", err))
	}

	const listQ = `
		SELECT id, tb_transfer_id, debit_account_id, credit_account_id,
		       amount_cents, code, COALESCE(description, ''), created_at
		FROM transactions_log
		WHERE debit_account_id = $1 OR credit_account_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := s.db.QueryContext(ctx, listQ, tbAccountID, limit, offset)
	if err != nil {
		return nil, 0, apperr.Internal(fmt.Errorf("list transactions: %w", err))
	}
	defer rows.Close()

	var out []*models.Transaction
	for rows.Next() {
		var t models.Transaction
		if err := rows.Scan(
			&t.ID, &t.TBTransferID, &t.DebitAccountID, &t.CreditAccountID,
			&t.AmountCents, &t.Code, &t.Description, &t.CreatedAt,
		); err != nil {
			return nil, 0, apperr.Internal(fmt.Errorf("scan transaction: %w", err))
		}
		t.FillHex()
		out = append(out, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, apperr.Internal(fmt.Errorf("rows error: %w", err))
	}

	return out, total, nil
}

// --- Pending confirmations ---

// SavePendingConfirmation guarda una operación pendiente de confirmación.
func (s *PostgresStore) SavePendingConfirmation(
	ctx context.Context,
	userID string,
	operationJSON []byte,
	expiresAt time.Time,
) (string, time.Time, error) {
	const q = `
		INSERT INTO pending_confirmations (user_id, operation, expires_at)
		VALUES ($1, $2, $3)
		RETURNING token, expires_at`

	var token string
	var exp time.Time
	err := s.db.QueryRowContext(ctx, q, userID, operationJSON, expiresAt).Scan(&token, &exp)
	if err != nil {
		return "", time.Time{}, apperr.Internal(fmt.Errorf("insert pending confirmation: %w", err))
	}
	return token, exp, nil
}

// ConsumePendingConfirmation recupera y marca como usada una operación pendiente.
func (s *PostgresStore) ConsumePendingConfirmation(
	ctx context.Context,
	token, userID string,
) ([]byte, error) {
	const q = `
		UPDATE pending_confirmations
		SET used_at = NOW()
		WHERE token = $1
		  AND user_id = $2
		  AND used_at IS NULL
		  AND expires_at > NOW()
		RETURNING operation`

	var operationJSON []byte
	err := s.db.QueryRowContext(ctx, q, token, userID).Scan(&operationJSON)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound(
				"CONFIRMATION_NOT_FOUND",
				"Operación pendiente no encontrada o expirada",
			)
		}
		return nil, apperr.Internal(fmt.Errorf("consume pending confirmation: %w", err))
	}
	return operationJSON, nil
}

// --- Helpers ---

func (s *PostgresStore) userExists(ctx context.Context, id string) (bool, error) {
	const q = `SELECT 1 FROM users WHERE id = $1`
	var dummy int
	err := s.db.QueryRowContext(ctx, q, id).Scan(&dummy)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, apperr.Internal(fmt.Errorf("user exists: %w", err))
	}
	return true, nil
}

func (s *PostgresStore) scanUser(row *sql.Row) (*models.User, error) {
	var u models.User
	err := row.Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.FullName, &u.Alias,
		&u.TBAccountID, &u.Status, &u.CreatedAt, &u.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("USER_NOT_FOUND", "Usuario no encontrado")
	}
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("scan user: %w", err))
	}
	return &u, nil
}
