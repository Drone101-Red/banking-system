package tigerbeetle

import (
	"fmt"
	"strconv"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"

	"banking-system/internal/models"
)

const (
	maxConnectionAttempts = 5
	connectionRetryDelay  = 2 * time.Second
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

// CreateAccount crea una cuenta de usuario con la configuración estándar:
// Ledger = LedgerUSD, Code = code, Flags = DebitsMustNotExceedCredits.
//
// Es idempotente:
//   - Si la cuenta no existe, la crea.
//   - Si ya existe con la configuración esperada, no hace nada.
//   - Si existe con configuración distinta (flags, ledger, code),
//     devuelve un error de integridad.
//
// TigerBeetle distingue estos casos en el status del CreateAccountResult,
// por lo que no se necesita un LookupAccounts previo.
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
			// Idempotente: la cuenta ya existe con la configuración esperada.
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

// u128Hex devuelve la representación hex de un Uint128 para logs de error.
// Evita imprimir con %v (que puede no ser legible).
func u128Hex(id tb.Uint128) string {
	b := id.Bytes()
	return fmt.Sprintf("%x", b)
}
