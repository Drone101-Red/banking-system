-- Extensiones
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Tabla de usuarios
CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    full_name     VARCHAR(255) NOT NULL,
    tb_account_id BYTEA NOT NULL UNIQUE,
    created_at    TIMESTAMP DEFAULT NOW(),
    updated_at    TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_tb_account_id ON users(tb_account_id);

-- Índice de lectura para el historial.
-- La verdad financiera está en TigerBeetle; esta tabla es solo un índice
-- de lectura para responder /history rápido sin paginar el ledger completo.
CREATE TABLE IF NOT EXISTS transactions_log (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tb_transfer_id    BYTEA NOT NULL UNIQUE,
    debit_account_id  BYTEA NOT NULL,
    credit_account_id BYTEA NOT NULL,
    amount_cents      BIGINT NOT NULL,
    code              SMALLINT NOT NULL,
    created_at        TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_txlog_debit   ON transactions_log(debit_account_id);
CREATE INDEX IF NOT EXISTS idx_txlog_credit  ON transactions_log(credit_account_id);
CREATE INDEX IF NOT EXISTS idx_txlog_created ON transactions_log(created_at DESC);