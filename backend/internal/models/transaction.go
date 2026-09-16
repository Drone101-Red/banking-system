package models

import (
	"encoding/hex"
	"time"
)

// Códigos de transferencia (Transfer.code en TigerBeetle).
const (
	TransferCodeDeposit      uint16 = 1
	TransferCodeWithdrawal   uint16 = 2
	TransferCodeUserTransfer uint16 = 3
)

// Transaction representa una transferencia registrada en el log de
// PostgreSQL (tabla transactions_log).
//
// NO es la fuente de verdad financiera. La fuente de verdad es
// TigerBeetle. Esta estructura es un índice de lectura que permite
// responder /history rápido sin paginar el ledger completo.
type Transaction struct {
	ID              string    `json:"id"`
	TBTransferID    []byte    `json:"-"` // uint128 como 16 bytes
	DebitAccountID  []byte    `json:"-"` // uint128 como 16 bytes
	CreditAccountID []byte    `json:"-"` // uint128 como 16 bytes
	DebitHex        string    `json:"debit_account_id"`
	CreditHex       string    `json:"credit_account_id"`
	AmountCents     int64     `json:"amount_cents"`
	Code            uint16    `json:"code"`
	Description     string    `json:"description,omitempty"`
	CreatedAt       time.Time `json:"created_at"`

	// IdempotencyKey es la clave enviada por el cliente para garantizar
	// idempotencia. NO se persiste en PostgreSQL. Se usa solo para
	// generar el TBTransferID de forma determinística.
	IdempotencyKey string `json:"-"`

	// FixtureCreatedAt es opcional. Si está seteado, se usa como
	// created_at en transactions_log en vez de NOW().
	// Uso exclusivo del seed de fixtures.
	FixtureCreatedAt *time.Time `json:"-"`
}

// FillHex llena los campos DebitHex y CreditHex a partir de los bytes.
func (t *Transaction) FillHex() {
	t.DebitHex = hex.EncodeToString(t.DebitAccountID)
	t.CreditHex = hex.EncodeToString(t.CreditAccountID)
}

// Direction describe el sentido de la transacción para el usuario.
type Direction string

const (
	DirectionIn  Direction = "in"  // el usuario recibió dinero
	DirectionOut Direction = "out" // el usuario envió dinero
)

// DirectionFor devuelve el sentido de la transacción para el usuario
// cuyo TBAccountID es `userTBID`.
func (t *Transaction) DirectionFor(userTBID []byte) Direction {
	if bytesEqual(t.CreditAccountID, userTBID) {
		return DirectionIn
	}
	return DirectionOut
}

// bytesEqual compara dos slices de bytes.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
