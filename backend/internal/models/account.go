package models

// AccountInfo es la representación pública de una cuenta de TigerBeetle.
//
// Se usa en:
//   - GET /api/account (información de la cuenta del usuario)
//   - GET /api/account/balance (solo el saldo)
//   - GET /api/transactions/history (para mostrar el saldo actual)
type AccountInfo struct {
	TBAccountID  string `json:"tb_account_id"` // hex del uint128 (32 chars)
	BalanceCents int64  `json:"balance_cents"` // saldo en centavos
	Ledger       uint32 `json:"ledger"`        // ledger (1 = USD)
	Code         uint16 `json:"code"`          // 100 = savings, 200 = bank
}
