package transactions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strings"

	tb "github.com/tigerbeetle/tigerbeetle-go"

	"banking-system/internal/apperr"
	"banking-system/internal/db"
	"banking-system/internal/models"
)

const (
	// DemoTopupAmountCents es el monto del crédito de demo ($1000).
	DemoTopupAmountCents int64 = 100_000
)

// TBClient abstrae las operaciones de TigerBeetle.
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

type TransferRequest struct {
	AmountCents int64 `json:"amount_cents"`
}

type UserTransferRequest struct {
	ToAccountID string `json:"to_account_id"`
	AmountCents int64  `json:"amount_cents"`
}

type HistoryResult struct {
	Transactions []*models.Transaction `json:"transactions"`
	Total        int                   `json:"total"`
	Page         int                   `json:"page"`
	Limit        int                   `json:"limit"`
	TotalPages   int                   `json:"total_pages"`
}

// DemoTopupResult es la respuesta de POST /api/transactions/demo-topup.
type DemoTopupResult struct {
	AmountCents int64               `json:"amount_cents"`
	Message     string              `json:"message"`
	Transaction *models.Transaction `json:"transaction"`
}

// --- Operaciones ---

// DemoTopup carga un crédito de demo de $1000.
//
// Solo disponible en APP_ENV=development.
func (s *Service) DemoTopup(ctx context.Context, userID string) (*DemoTopupResult, error) {
	user, err := s.getActiveUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	bankID := tb.ToUint128(models.BankAccountID)
	userTBID := userTBAccountID(user)

	tx := &models.Transaction{
		DebitAccountID:  u128ToBytes(bankID),
		CreditAccountID: u128ToBytes(userTBID),
		AmountCents:     DemoTopupAmountCents,
		Code:            models.TransferCodeDeposit,
		IdempotencyKey:  deriveIdempotencyKey(userID, "demo-topup"),
	}

	if err := s.executeTransfer(ctx, tx); err != nil {
		return nil, err
	}

	return &DemoTopupResult{
		AmountCents: DemoTopupAmountCents,
		Message:     "Crédito de demo cargado",
		Transaction: tx,
	}, nil
}

// Deposit agrega fondos a la cuenta del usuario.
func (s *Service) Deposit(
	ctx context.Context,
	userID string,
	amountCents int64,
	idempotencyKey string,
) (*models.Transaction, error) {
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
		IdempotencyKey:  deriveIdempotencyKey(userID, idempotencyKey),
	}

	if err := s.executeTransfer(ctx, tx); err != nil {
		return nil, err
	}
	return tx, nil
}

// Withdraw retira fondos de la cuenta del usuario.
func (s *Service) Withdraw(
	ctx context.Context,
	userID string,
	amountCents int64,
	idempotencyKey string,
) (*models.Transaction, error) {
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
		IdempotencyKey:  deriveIdempotencyKey(userID, idempotencyKey),
	}

	if err := s.executeTransfer(ctx, tx); err != nil {
		return nil, err
	}
	return tx, nil
}

// Transfer envía fondos a otra cuenta.
func (s *Service) Transfer(
	ctx context.Context,
	userID string,
	toAccountHex string,
	amountCents int64,
	idempotencyKey string,
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

	dst, err := s.pg.GetUserByTBAccountID(ctx, toBytes)
	if err != nil {
		if appErr, ok := err.(*apperr.AppError); ok && appErr.Code == "USER_NOT_FOUND" {
			return nil, apperr.NotFound("DEST_NOT_FOUND", "Cuenta destino no encontrada")
		}
		return nil, err
	}
	if dst.Status != models.StatusActive {
		return nil, apperr.BadRequest("DEST_NOT_ACTIVE", "La cuenta destino no está activa")
	}

	if bytesEqual(src.TBAccountID, dst.TBAccountID) {
		return nil, apperr.BadRequest("SAME_ACCOUNT", "Origen y destino no pueden ser la misma cuenta")
	}

	tx := &models.Transaction{
		DebitAccountID:  src.TBAccountID,
		CreditAccountID: dst.TBAccountID,
		AmountCents:     amountCents,
		Code:            models.TransferCodeUserTransfer,
		IdempotencyKey:  deriveIdempotencyKey(userID, idempotencyKey),
	}

	if err := s.executeTransfer(ctx, tx); err != nil {
		return nil, err
	}
	return tx, nil
}

// History devuelve el historial paginado del usuario.
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

func deriveIdempotencyKey(userID, clientKey string) string {
	if clientKey == "" {
		return ""
	}
	h := sha256.New()
	h.Write([]byte(userID))
	h.Write([]byte{0x00})
	h.Write([]byte(clientKey))
	return hex.EncodeToString(h.Sum(nil))
}

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

func (s *Service) executeTransfer(ctx context.Context, tx *models.Transaction) error {
	transferID, err := s.tb.CreateTransfer(tx)
	if err != nil {
		return err
	}
	tx.TBTransferID = transferID

	if err := s.pg.InsertTransactionLog(ctx, tx); err != nil {
		log.Printf("[tx] transferencia %s ejecutada en TB pero log falló: %v",
			hex.EncodeToString(transferID), err)
	}

	tx.FillHex()
	return nil
}

func validateAmount(amountCents int64) error {
	if amountCents <= 0 {
		return apperr.BadRequest("INVALID_AMOUNT", "El monto debe ser mayor a cero")
	}
	return nil
}

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

func userTBAccountID(u *models.User) tb.Uint128 {
	var arr [16]byte
	copy(arr[:], u.TBAccountID)
	return tb.BytesToUint128(arr)
}

func u128ToBytes(id tb.Uint128) []byte {
	b := id.Bytes()
	return b[:]
}

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
