package tigerbeetle

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strconv"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"

	"banking-system/internal/apperr"
	"banking-system/internal/models"
)

const (
	maxConnectionAttempts = 5
	connectionRetryDelay  = 2 * time.Second

	// accountLookupTimeout acota el tiempo máximo que esperamos por un
	// LookupAccounts. El cliente TigerBeetle no acepta context.Context y
	// no tiene timeout configurable, así que el timeout se aplica desde
	// afuera, con una goroutine y un select sobre time.After.
	accountLookupTimeout = 5 * time.Second
)

type Client struct {
	client tb.Client
}

func New(address string, clusterID string) (*Client, error) {
	id, err := strconv.ParseUint(clusterID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("cluster ID inválido: %w", err)
	}

	var connectedClient *Client

	err = retry(maxConnectionAttempts, connectionRetryDelay, func() error {
		client, err := tb.NewClient(
			tb.ToUint128(id),
			[]string{address},
		)
		if err != nil {
			return fmt.Errorf("crear cliente TigerBeetle: %w", err)
		}

		candidate := &Client{
			client: client,
		}

		if err := candidate.Ping(); err != nil {
			candidate.Close()
			return err
		}

		connectedClient = candidate
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf(
			"TigerBeetle no disponible después de %d intentos: %w",
			maxConnectionAttempts,
			err,
		)
	}

	return connectedClient, nil
}

func (c *Client) Ping() error {
	if c == nil || c.client == nil {
		return fmt.Errorf("cliente TigerBeetle no inicializado")
	}

	if err := c.client.Nop(); err != nil {
		return fmt.Errorf("ping TigerBeetle: %w", err)
	}

	return nil
}

func (c *Client) Close() {
	if c == nil || c.client == nil {
		return
	}

	c.client.Close()
}

// AccountExists devuelve true si la cuenta con el ID dado existe en TB.
//
// Aplica un timeout porque el cliente TigerBeetle no respeta context.Context
// y no tiene timeout configurable.
func (c *Client) AccountExists(id tb.Uint128) (bool, error) {
	if c == nil || c.client == nil {
		return false, fmt.Errorf("cliente TigerBeetle no inicializado")
	}

	type result struct {
		exists bool
		err    error
	}
	ch := make(chan result, 1)

	go func() {
		accounts, err := c.client.LookupAccounts([]tb.Uint128{id})
		if err != nil {
			ch <- result{false, fmt.Errorf("lookup account %s: %w", u128Hex(id), err)}
			return
		}
		ch <- result{len(accounts) > 0, nil}
	}()

	select {
	case r := <-ch:
		return r.exists, r.err
	case <-time.After(accountLookupTimeout):
		return false, fmt.Errorf("lookup account %s: timeout after %v", u128Hex(id), accountLookupTimeout)
	}
}

// CreateAccount crea una cuenta de usuario con la configuración estándar.
func (c *Client) CreateAccount(id tb.Uint128, code uint16) error {
	if c == nil || c.client == nil {
		return fmt.Errorf("cliente TigerBeetle no inicializado")
	}

	flags := tb.AccountFlags{
		DebitsMustNotExceedCredits: true,
	}.ToUint16()

	res, err := c.client.CreateAccounts([]tb.Account{{
		ID:     id,
		Ledger: models.LedgerUSD,
		Code:   code,
		Flags:  flags,
	}})
	if err != nil {
		return fmt.Errorf("create account %s: %w", u128Hex(id), err)
	}

	for _, r := range res {
		switch r.Status {
		case tb.AccountCreated:
			return nil
		case tb.AccountExists:
			return nil
		case tb.AccountExistsWithDifferentFlags:
			return fmt.Errorf("account %s exists with different flags", u128Hex(id))
		case tb.AccountExistsWithDifferentLedger:
			return fmt.Errorf("account %s exists with different ledger", u128Hex(id))
		case tb.AccountExistsWithDifferentCode:
			return fmt.Errorf("account %s exists with different code (expected %d)", u128Hex(id), code)
		case tb.AccountIDMustNotBeZero:
			return fmt.Errorf("account ID must not be zero")
		default:
			return fmt.Errorf("unexpected create account status for %s: %s", u128Hex(id), r.Status)
		}
	}

	return nil
}

// EnsureBankAccount garantiza que la cuenta bancaria del sistema exista.
func (c *Client) EnsureBankAccount() error {
	if c == nil || c.client == nil {
		return fmt.Errorf("cliente TigerBeetle no inicializado")
	}

	bankID := tb.ToUint128(models.BankAccountID)

	res, err := c.client.CreateAccounts([]tb.Account{{
		ID:     bankID,
		Ledger: models.LedgerUSD,
		Code:   models.CodeBank,
		Flags:  0,
	}})
	if err != nil {
		return fmt.Errorf("ensure bank account %s: %w", u128Hex(bankID), err)
	}

	for _, r := range res {
		switch r.Status {
		case tb.AccountCreated:
			return nil
		case tb.AccountExists:
			return nil
		case tb.AccountExistsWithDifferentFlags:
			return fmt.Errorf("bank account %s exists with different flags", u128Hex(bankID))
		case tb.AccountExistsWithDifferentLedger:
			return fmt.Errorf("bank account %s exists with different ledger", u128Hex(bankID))
		case tb.AccountExistsWithDifferentCode:
			return fmt.Errorf("bank account %s exists with different code", u128Hex(bankID))
		case tb.AccountIDMustNotBeZero:
			return fmt.Errorf("bank account ID must not be zero")
		default:
			return fmt.Errorf("unexpected create bank account status: %s", r.Status)
		}
	}

	return nil
}

// CreateTransfer ejecuta una transferencia en TigerBeetle.
//
// El struct Transaction debe tener:
//   - TBTransferID (opcional, si nil se genera uno)
//   - DebitAccountID (16 bytes)
//   - CreditAccountID (16 bytes)
//   - AmountCents (positivo)
//   - Code (TransferCodeDeposit | Withdrawal | UserTransfer)
//
// Devuelve el TBTransferID de la transferencia creada.
func (c *Client) CreateTransfer(t *models.Transaction) ([]byte, error) {
	if c == nil || c.client == nil {
		return nil, apperr.Internal(fmt.Errorf("cliente TigerBeetle no inicializado"))
	}
	if len(t.DebitAccountID) != 16 {
		return nil, apperr.Internal(fmt.Errorf("DebitAccountID debe ser 16 bytes"))
	}
	if len(t.CreditAccountID) != 16 {
		return nil, apperr.Internal(fmt.Errorf("CreditAccountID debe ser 16 bytes"))
	}
	if t.AmountCents <= 0 {
		return nil, apperr.BadRequest("INVALID_AMOUNT", "El monto debe ser positivo")
	}

	transferID, transferIDBytes, err := generateTransferID(
		t.TBTransferID,
		t.IdempotencyKey,
	)

	res, err := c.client.CreateTransfers([]tb.Transfer{{
		ID:              transferID,
		DebitAccountID:  bytesToU128(t.DebitAccountID),
		CreditAccountID: bytesToU128(t.CreditAccountID),
		Amount:          tb.ToUint128(uint64(t.AmountCents)),
		Ledger:          models.LedgerUSD,
		Code:            t.Code,
	}})
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("create transfer: %w", err))
	}

	for _, r := range res {
		switch r.Status {
		case tb.TransferCreated:
			return transferIDBytes, nil
		case tb.TransferExists:
			return transferIDBytes, nil
		case tb.TransferExceedsCredits:
			return nil, apperr.BadRequest("INSUFFICIENT_FUNDS", "Saldo insuficiente")
		case tb.TransferDebitAccountNotFound:
			return nil, apperr.BadRequest("SOURCE_NOT_FOUND", "Cuenta origen no encontrada")
		case tb.TransferCreditAccountNotFound:
			return nil, apperr.BadRequest("DEST_NOT_FOUND", "Cuenta destino no encontrada")
		case tb.TransferAccountsMustBeDifferent:
			return nil, apperr.BadRequest("SAME_ACCOUNT", "Origen y destino no pueden ser la misma cuenta")
		case tb.TransferLedgerMustNotBeZero:
			return nil, apperr.Internal(fmt.Errorf("ledger inválido"))
		case tb.TransferCodeMustNotBeZero:
			return nil, apperr.Internal(fmt.Errorf("code inválido"))
		default:
			return nil, apperr.Internal(fmt.Errorf("transfer status: %s", r.Status))
		}
	}

	return nil, apperr.Internal(fmt.Errorf("transfer: respuesta vacía"))
}

// GetBalance devuelve el saldo de una cuenta en centavos.
//
// El saldo se calcula como credits_posted - debits_posted.
// Para cuentas de usuario (DebitsMustNotExceedCredits), siempre >= 0.
func (c *Client) GetBalance(id tb.Uint128) (int64, error) {
	if c == nil || c.client == nil {
		return 0, apperr.Internal(fmt.Errorf("cliente TigerBeetle no inicializado"))
	}

	type result struct {
		balance int64
		err     error
	}
	ch := make(chan result, 1)

	go func() {
		accounts, err := c.client.LookupAccounts([]tb.Uint128{id})
		if err != nil {
			ch <- result{0, apperr.Internal(fmt.Errorf("lookup account: %w", err))}
			return
		}
		if len(accounts) == 0 {
			ch <- result{0, apperr.NotFound("ACCOUNT_NOT_FOUND", "Cuenta no encontrada")}
			return
		}
		acc := accounts[0]
		credits := uint128ToInt64(acc.CreditsPosted)
		debits := uint128ToInt64(acc.DebitsPosted)
		ch <- result{credits - debits, nil}
	}()

	select {
	case r := <-ch:
		return r.balance, r.err
	case <-time.After(accountLookupTimeout):
		return 0, apperr.Internal(fmt.Errorf("get balance timeout after %v", accountLookupTimeout))
	}
}

// GetAccountInfo devuelve la información completa de una cuenta.
func (c *Client) GetAccountInfo(id tb.Uint128) (*models.AccountInfo, error) {
	if c == nil || c.client == nil {
		return nil, apperr.Internal(fmt.Errorf("cliente TigerBeetle no inicializado"))
	}

	type result struct {
		info *models.AccountInfo
		err  error
	}
	ch := make(chan result, 1)

	go func() {
		accounts, err := c.client.LookupAccounts([]tb.Uint128{id})
		if err != nil {
			ch <- result{nil, apperr.Internal(fmt.Errorf("lookup account: %w", err))}
			return
		}
		if len(accounts) == 0 {
			ch <- result{nil, apperr.NotFound("ACCOUNT_NOT_FOUND", "Cuenta no encontrada")}
			return
		}
		acc := accounts[0]
		credits := uint128ToInt64(acc.CreditsPosted)
		debits := uint128ToInt64(acc.DebitsPosted)
		ch <- result{
			&models.AccountInfo{
				TBAccountID:  u128Hex(acc.ID),
				BalanceCents: credits - debits,
				Ledger:       acc.Ledger,
				Code:         acc.Code,
			},
			nil,
		}
	}()

	select {
	case r := <-ch:
		return r.info, r.err
	case <-time.After(accountLookupTimeout):
		return nil, apperr.Internal(fmt.Errorf("get account info timeout after %v", accountLookupTimeout))
	}
}

// --- Helpers ---

// u128Hex devuelve la representación hex de un Uint128 para logs de error.
//
// TigerBeetle serializa Uint128 en little-endian, así que el hex resultante
// es consistente con el que se guarda en PostgreSQL (BYTEA = Bytes() crudo).
func u128Hex(id tb.Uint128) string {
	b := id.Bytes()
	return fmt.Sprintf("%x", b)
}

// bytesToU128 convierte 16 bytes a Uint128.
func bytesToU128(b []byte) tb.Uint128 {
	var arr [16]byte
	copy(arr[:], b)
	return tb.BytesToUint128(arr)
}

// uint128ToInt64 extrae el valor como int64.
//
// TigerBeetle serializa Uint128 en little-endian: el byte 0 es el menos
// significativo. Los saldos en centavos caben holgadamente en los
// primeros 8 bytes.
//
// Verificado empíricamente:
//
//	tb.ToUint128(12000).Bytes() = e02e0000...  (little-endian)
func uint128ToInt64(u tb.Uint128) int64 {
	b := u.Bytes()
	return int64(binary.LittleEndian.Uint64(b[:8]))
}
func generateTransferID(
	provided []byte,
	idempotencyKey string,
) (tb.Uint128, []byte, error) {
	if len(provided) == 16 {
		return bytesToU128(provided), provided, nil
	}

	if idempotencyKey != "" {
		h := sha256.New()
		h.Write([]byte(idempotencyKey))
		sum := h.Sum(nil)
		var b [16]byte
		copy(b[:], sum[:16])
		return tb.BytesToUint128(b), b[:], nil
	}

	// Fallback: timestamp + random
	var b [16]byte
	now := time.Now().UnixNano()
	for i := 0; i < 8; i++ {
		b[i] = byte(now >> (56 - i*8))
	}
	if _, err := rand.Read(b[8:]); err != nil {
		return tb.Uint128{}, nil, fmt.Errorf("rand.Read: %w", err)
	}
	return tb.BytesToUint128(b), b[:], nil
}
