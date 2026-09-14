// Package models contiene los tipos y constantes compartidos del dominio.
//
// Este archivo define exclusivamente las constantes financieras del sistema.
// Su objetivo es evitar números mágicos repartidos por el código y tener un
// único origen de verdad para los valores que configuran las cuentas y
// transferencias en TigerBeetle.
//
// Los valores aquí definidos corresponden a decisiones de dominio, no a
// configuración del entorno. Cambiarlos implica migración de datos en
// TigerBeetle, por lo que se consideran inmutables en runtime.
package models

const (
	// LedgerUSD identifica el libro contable único del sistema.
	// Todas las cuentas (usuarios y banco) pertenecen a este ledger.
	LedgerUSD uint32 = 1

	// BankAccountID es el ID reservado de la cuenta bancaria del sistema.
	// Es la contrapartida externa para depósitos y retiros.
	// Se almacena como uint64 (configuración); se convierte a Uint128
	// de TigerBeetle con tigerbeetle.ToUint128 en el momento de uso.
	BankAccountID uint64 = 1

	// CodeSavings identifica cuentas de ahorro de usuario.
	// Las cuentas con este code tienen el flag DebitsMustNotExceedCredits.
	CodeSavings uint16 = 100

	// CodeBank identifica la cuenta bancaria del sistema.
	// Es la única cuenta con CodeCodeBank y sin restricción de débito.
	CodeBank uint16 = 200
)
