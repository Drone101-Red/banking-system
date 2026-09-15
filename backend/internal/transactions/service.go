// Package transactions implementa la lógica de negocio de las
// operaciones financieras: depósito, retiro, transferencia y historial.
//
// Todas las operaciones se ejecutan contra TigerBeetle, que es la
// fuente de verdad financiera. Después de cada operación exitosa se
// escribe un índice en PostgreSQL (transactions_log) para responder
// el historial rápido.
package transactions

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"strings"

	tb "github.com/tigerbeetle/tigerbeetle-go"

	"banking-system/internal/apperr"
	"banking-system/internal/db"
	"banking-system/internal/models"
)

// TBClient abstrae las operaciones de TigerBeetle que necesita el Service.
// Permite mockear en tests unitarios.
type TBClient interface {
	AccountExists(id tb.Uint128) (bool, error)
	CreateTransfer(t *models.Transaction) ([]byte, error)
	GetBalance(id tb.Uint128) (int64, error)
	GetAccountInfo(id tb.Uint128) (*models.AccountInfo, error)
}

// Service orquesta las operaciones financieras.
type Service struct {
	pg *db.PostgresStore
	tb TBClient
}

// NewService construye el Service.
func NewService(pg *db.PostgresStore, tb TBClient) *Service {
	return &Service{pg: pg, tb: tb}
}

// --- Requests / Results ---

// TransferRequest es el DTO de entrada de POST /api/transactions/deposit
// y /withdraw.
type TransferRequest struct {
	AmountCents int64 `json:"amount_cents"`
}

// UserTransferRequest es el DTO de entrada de
// POST /api/transactions/transfer.
type UserTransferRequest struct {
	ToAccountID string `json:"to_account_id"` // hex del uint128 destino
	AmountCents int64  `json:"amount_cents"`
}

// HistoryResult es la respuesta paginada del historial.
type HistoryResult struct {
	Transactions []*models.Transaction `json:"transactions"`
	Total        int                   `json:"total"`
	Page         int                   `json:"page"`
	Limit        int                   `json:"limit"`
	TotalPages   int                   `json:"total_pages"`
}

// --- Operaciones ---

// Deposit agrega fondos a la cuenta del usuario.
//
// Flujo:
//  1. Validar amountCents > 0.
//  2. Buscar usuario.
//  3. Crear transferencia banco -> usuario.
//  4. Registrar en transactions_log.
func (s *Service) Deposit(ctx context.Context, userID string, amountCents int64) (*models.Transaction, error) {
	if err := validateAmount(amountCents); err != nil {
		return nil, err
	}

	user, err := s.getActiveUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	bankID := tb.ToUint128(models.BankAccountID)
	userTBID := userTBAccountID(user)

	tx := &models.Transaction{
		DebitAccountID:  u128ToBytes(bankID),
		CreditAccountID: u128ToBytes(userTBID),
		AmountCents:     amountCents,
		Code:            models.TransferCodeDeposit,
	}

	if err := s.executeTransfer(ctx, tx); err != nil {
		return nil, err
	}
	return tx, nil
}

// Withdraw retira fondos de la cuenta del usuario.
//
// Flujo:
//  1. Validar amountCents > 0.
//  2. Buscar usuario.
//  3. Crear transferencia usuario -> banco.
//  4. TigerBeetle valida saldo suficiente.
//  5. Registrar en transactions_log.
func (s *Service) Withdraw(ctx context.Context, userID string, amountCents int64) (*models.Transaction, error) {
	if err := validateAmount(amountCents); err != nil {
		return nil, err
	}

	user, err := s.getActiveUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	bankID := tb.ToUint128(models.BankAccountID)
	userTBID := userTBAccountID(user)

	tx := &models.Transaction{
		DebitAccountID:  u128ToBytes(userTBID),
		CreditAccountID: u128ToBytes(bankID),
		AmountCents:     amountCents,
		Code:            models.TransferCodeWithdrawal,
	}

	if err := s.executeTransfer(ctx, tx); err != nil {
		return nil, err
	}
	return tx, nil
}

// Transfer envía fondos a otra cuenta.
//
// Flujo:
//  1. Validar amountCents > 0.
//  2. Decodificar toAccountHex.
//  3. Buscar usuario origen.
//  4. Buscar usuario destino (debe estar ACTIVE).
//  5. Validar que origen != destino.
//  6. Crear transferencia origen -> destino.
//  7. Registrar en transactions_log.
func (s *Service) Transfer(
	ctx context.Context,
	userID string,
	toAccountHex string,
	amountCents int64,
) (*models.Transaction, error) {
	if err := validateAmount(amountCents); err != nil {
		return nil, err
	}

	toBytes, err := parseHex16(toAccountHex)
	if err != nil {
		return nil, apperr.BadRequest("INVALID_DEST_ACCOUNT", "Identificador de cuenta destino inválido")
	}

	src, err := s.getActiveUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Validar que el destino exista y esté ACTIVE.
	dst, err := s.pg.GetUserByTBAccountID(ctx, toBytes)
	if err != nil {
		// Si no existe, apperr.NotFound. Lo traducimos a algo más
		// específico del contexto de transferencia.
		if appErr, ok := err.(*apperr.AppError); ok && appErr.Code == "USER_NOT_FOUND" {
			return nil, apperr.NotFound("DEST_NOT_FOUND", "Cuenta destino no encontrada")
		}
		return nil, err
	}
	if dst.Status != models.StatusActive {
		return nil, apperr.BadRequest("DEST_NOT_ACTIVE", "La cuenta destino no está activa")
	}

	// Validar que no sea la misma cuenta.
	if bytesEqual(src.TBAccountID, dst.TBAccountID) {
		return nil, apperr.BadRequest("SAME_ACCOUNT", "Origen y destino no pueden ser la misma cuenta")
	}

	tx := &models.Transaction{
		DebitAccountID:  src.TBAccountID,
		CreditAccountID: dst.TBAccountID,
		AmountCents:     amountCents,
		Code:            models.TransferCodeUserTransfer,
	}

	if err := s.executeTransfer(ctx, tx); err != nil {
		return nil, err
	}
	return tx, nil
}

// History devuelve el historial paginado del usuario.
//
// Flujo:
//  1. Buscar usuario.
//  2. ListTransactions con limit/offset.
//  3. Calcular total_pages.
func (s *Service) History(ctx context.Context, userID string, page, limit int) (*HistoryResult, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	user, err := s.getActiveUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	offset := (page - 1) * limit
	txs, total, err := s.pg.ListTransactions(ctx, user.TBAccountID, limit, offset)
	if err != nil {
		return nil, err
	}

	totalPages := (total + limit - 1) / limit

	return &HistoryResult{
		Transactions: txs,
		Total:        total,
		Page:         page,
		Limit:        limit,
		TotalPages:   totalPages,
	}, nil
}

// --- Helpers ---

// getActiveUser busca un usuario y valida que esté ACTIVE.
func (s *Service) getActiveUser(ctx context.Context, userID string) (*models.User, error) {
	user, err := s.pg.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.Status != models.StatusActive {
		return nil, apperr.Forbidden("USER_NOT_ACTIVE", "La cuenta no está activa")
	}
	return user, nil
}

// executeTransfer ejecuta la transferencia en TB y registra el log.
//
// Si TB rechaza la transferencia, devuelve el error (la operación NO
// se realizó).
//
// Si TB acepta pero InsertTransactionLog falla, se loggea el error y
// se devuelve éxito. La fuente de verdad es TigerBeetle: la
// transferencia SÍ se hizo. El log es un índice de lectura secundario.
//
// Después del insert, se llama FillHex para que la respuesta JSON
// incluya los campos derivados (debit_account_id, credit_account_id
// en formato hex).
func (s *Service) executeTransfer(ctx context.Context, tx *models.Transaction) error {
	transferID, err := s.tb.CreateTransfer(tx)
	if err != nil {
		return err
	}
	tx.TBTransferID = transferID

	if err := s.pg.InsertTransactionLog(ctx, tx); err != nil {
		log.Printf("[tx] transferencia %s ejecutada en TB pero log falló: %v",
			hex.EncodeToString(transferID), err)
		// No devolvemos error: el dinero ya se movió.
	}

	// Llenar los campos hex para que la respuesta JSON sea completa.
	tx.FillHex()
	return nil
} // validateAmount valida que el monto sea positivo.
func validateAmount(amountCents int64) error {
	if amountCents <= 0 {
		return apperr.BadRequest("INVALID_AMOUNT", "El monto debe ser mayor a cero")
	}
	return nil
}

// parseHex16 decodifica un string hex de 32 caracteres a 16 bytes.
func parseHex16(s string) ([]byte, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if len(s) != 32 {
		return nil, fmt.Errorf("longitud esperada 32, obtuve %d", len(s))
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	if len(b) != 16 {
		return nil, fmt.Errorf("esperaba 16 bytes, obtuve %d", len(b))
	}
	return b, nil
}

// userTBAccountID devuelve el Uint128 del user.
func userTBAccountID(u *models.User) tb.Uint128 {
	var arr [16]byte
	copy(arr[:], u.TBAccountID)
	return tb.BytesToUint128(arr)
}

// u128ToBytes convierte un Uint128 a 16 bytes.
func u128ToBytes(id tb.Uint128) []byte {
	b := id.Bytes()
	return b[:]
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
