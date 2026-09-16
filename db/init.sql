-- Extensiones
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Tabla de usuarios
--
-- Estados:
--   PENDING: usuario creado, cuenta TigerBeetle aún no confirmada
--   ACTIVE:  usuario operativo, cuenta TigerBeetle confirmada
--
-- alias: identificador corto para transferencias (ej: "juan", "maria2").
--        Único, generado automáticamente en el registro.
CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    full_name     VARCHAR(255) NOT NULL,
    alias         VARCHAR(50) UNIQUE NOT NULL,
    tb_account_id BYTEA NOT NULL UNIQUE,
    status        VARCHAR(20) NOT NULL DEFAULT 'PENDING'
                  CHECK (status IN ('PENDING', 'ACTIVE')),
    created_at    TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_alias ON users(alias);
CREATE INDEX IF NOT EXISTS idx_users_tb_account_id ON users(tb_account_id);
CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);

-- Índice de lectura para el historial.
CREATE TABLE IF NOT EXISTS transactions_log (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tb_transfer_id    BYTEA NOT NULL UNIQUE,
    debit_account_id  BYTEA NOT NULL,
    credit_account_id BYTEA NOT NULL,
    amount_cents      BIGINT NOT NULL,
    code              SMALLINT NOT NULL,
    description       VARCHAR(255),
    created_at        TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_txlog_debit   ON transactions_log(debit_account_id);
CREATE INDEX IF NOT EXISTS idx_txlog_credit  ON transactions_log(credit_account_id);
CREATE INDEX IF NOT EXISTS idx_txlog_created ON transactions_log(created_at DESC);

-- Operaciones pendientes de confirmación (chat IA).
CREATE TABLE IF NOT EXISTS pending_confirmations (
    token         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    operation     JSONB NOT NULL,
    expires_at    TIMESTAMP NOT NULL,
    used_at       TIMESTAMP,
    created_at    TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_pending_confirmations_user
    ON pending_confirmations(user_id)
    WHERE used_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_pending_confirmations_expires
    ON pending_confirmations(expires_at)
    WHERE used_at IS NULL;