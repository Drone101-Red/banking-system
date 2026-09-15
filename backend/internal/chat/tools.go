package chat

import (
	"context"
	"encoding/json"
	"fmt"

	"banking-system/internal/account"
	"banking-system/internal/apperr"
	"banking-system/internal/transactions"
)

// --- Definiciones de tools ---

type ToolDefinition struct {
	Type     string             `json:"type"`
	Function FunctionDefinition `json:"function"`
}

type FunctionDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

func Tools() []ToolDefinition {
	return []ToolDefinition{
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        "get_balance",
				Description: "Devuelve el saldo actual de la cuenta del usuario autenticado, en centavos.",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        "get_history",
				Description: "Devuelve las últimas transacciones del usuario autenticado. Por defecto devuelve las últimas 10.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"limit": map[string]any{
							"type":        "integer",
							"description": "Número máximo de transacciones a devolver (1-50).",
							"minimum":     1,
							"maximum":     50,
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        "deposit",
				Description: "Deposita dinero en la cuenta del usuario autenticado. SIEMPRE debes pedir confirmación al usuario antes de llamar esta tool.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"amount_cents": map[string]any{
							"type":        "integer",
							"description": "Cantidad a depositar en centavos. Por ejemplo, $10.50 = 1050.",
							"minimum":     1,
						},
					},
					"required": []string{"amount_cents"},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        "withdraw",
				Description: "Retira dinero de la cuenta del usuario autenticado. SIEMPRE debes pedir confirmación al usuario antes de llamar esta tool.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"amount_cents": map[string]any{
							"type":        "integer",
							"description": "Cantidad a retirar en centavos. Por ejemplo, $30.00 = 3000.",
							"minimum":     1,
						},
					},
					"required": []string{"amount_cents"},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        "transfer",
				Description: "Transfiere dinero a otra cuenta. SIEMPRE debes pedir confirmación al usuario antes de llamar esta tool.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"to_account_id": map[string]any{
							"type":        "string",
							"description": "Identificador de la cuenta destino (hex de 32 caracteres).",
							"pattern":     "^[0-9a-fA-F]{32}$",
						},
						"amount_cents": map[string]any{
							"type":        "integer",
							"description": "Cantidad a transferir en centavos.",
							"minimum":     1,
						},
					},
					"required": []string{"to_account_id", "amount_cents"},
				},
			},
		},
	}
}

// --- Ejecutores ---

// Executor ejecuta las tools contra los servicios del backend.
type Executor struct {
	accountSvc *account.Service
	txnSvc     *transactions.Service
}

// NewExecutor construye el Executor.
func NewExecutor(accountSvc *account.Service, txnSvc *transactions.Service) *Executor {
	return &Executor{
		accountSvc: accountSvc,
		txnSvc:     txnSvc,
	}
}

// Execute ejecuta la tool indicada.
//
// userID es el UUID del usuario autenticado (viene del JWT, NO del modelo).
// toolCallID es el ID de la llamada del modelo. Se usa como idempotency
// key para las tools de escritura: si el modelo pide la misma tool con
// el mismo ID, no se duplica la transferencia.
func (e *Executor) Execute(
	ctx context.Context,
	userID string,
	toolCallID string,
	name string,
	args map[string]any,
) (any, error) {
	switch name {
	case "get_balance":
		return e.execGetBalance(ctx, userID)
	case "get_history":
		return e.execGetHistory(ctx, userID, args)
	case "deposit":
		return e.execDeposit(ctx, userID, toolCallID, args)
	case "withdraw":
		return e.execWithdraw(ctx, userID, toolCallID, args)
	case "transfer":
		return e.execTransfer(ctx, userID, toolCallID, args)
	default:
		return nil, apperr.BadRequest("UNKNOWN_TOOL", fmt.Sprintf("Tool desconocida: %s", name))
	}
}

// --- Implementaciones ---

func (e *Executor) execGetBalance(ctx context.Context, userID string) (any, error) {
	balance, err := e.accountSvc.Balance(ctx, userID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"balance_cents":     balance,
		"balance_formatted": formatCents(balance),
	}, nil
}

func (e *Executor) execGetHistory(ctx context.Context, userID string, args map[string]any) (any, error) {
	limit := 10
	if v, ok := args["limit"]; ok {
		if n, ok := v.(float64); ok {
			limit = int(n)
		}
	}
	if limit < 1 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	result, err := e.txnSvc.History(ctx, userID, 1, limit)
	if err != nil {
		return nil, err
	}

	txs := make([]map[string]any, 0, len(result.Transactions))
	for _, tx := range result.Transactions {
		txs = append(txs, map[string]any{
			"id":               tx.ID,
			"amount_cents":     tx.AmountCents,
			"amount_formatted": formatCents(tx.AmountCents),
			"code":             tx.Code,
			"code_description": codeDescription(tx.Code),
			"created_at":       tx.CreatedAt,
		})
	}

	return map[string]any{
		"transactions": txs,
		"total":        result.Total,
	}, nil
}

func (e *Executor) execDeposit(
	ctx context.Context,
	userID string,
	toolCallID string,
	args map[string]any,
) (any, error) {
	amount, err := extractAmount(args)
	if err != nil {
		return nil, err
	}

	tx, err := e.txnSvc.Deposit(ctx, userID, amount, toolCallID)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"transaction_id":   tx.ID,
		"amount_cents":     tx.AmountCents,
		"amount_formatted": formatCents(tx.AmountCents),
		"message":          "Depósito realizado correctamente",
	}, nil
}

func (e *Executor) execWithdraw(
	ctx context.Context,
	userID string,
	toolCallID string,
	args map[string]any,
) (any, error) {
	amount, err := extractAmount(args)
	if err != nil {
		return nil, err
	}

	tx, err := e.txnSvc.Withdraw(ctx, userID, amount, toolCallID)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"transaction_id":   tx.ID,
		"amount_cents":     tx.AmountCents,
		"amount_formatted": formatCents(tx.AmountCents),
		"message":          "Retiro realizado correctamente",
	}, nil
}

func (e *Executor) execTransfer(
	ctx context.Context,
	userID string,
	toolCallID string,
	args map[string]any,
) (any, error) {
	toAccountID, ok := args["to_account_id"].(string)
	if !ok || toAccountID == "" {
		return nil, apperr.BadRequest("MISSING_TO_ACCOUNT_ID", "Falta el campo to_account_id")
	}

	amount, err := extractAmount(args)
	if err != nil {
		return nil, err
	}

	tx, err := e.txnSvc.Transfer(ctx, userID, toAccountID, amount, toolCallID)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"transaction_id":   tx.ID,
		"amount_cents":     tx.AmountCents,
		"amount_formatted": formatCents(tx.AmountCents),
		"to_account_id":    toAccountID,
		"message":          "Transferencia realizada correctamente",
	}, nil
}

// --- Helpers ---

func extractAmount(args map[string]any) (int64, error) {
	v, ok := args["amount_cents"]
	if !ok {
		return 0, apperr.BadRequest("MISSING_AMOUNT", "Falta el campo amount_cents")
	}
	f, ok := v.(float64)
	if !ok {
		return 0, apperr.BadRequest("INVALID_AMOUNT", "amount_cents debe ser un número")
	}
	amount := int64(f)
	if amount <= 0 {
		return 0, apperr.BadRequest("INVALID_AMOUNT", "amount_cents debe ser mayor a cero")
	}
	return amount, nil
}

func formatCents(cents int64) string {
	negative := cents < 0
	if cents < 0 {
		cents = -cents
	}
	dollars := cents / 100
	remainder := cents % 100
	sign := ""
	if negative {
		sign = "-"
	}
	return fmt.Sprintf("%s$%d.%02d", sign, dollars, remainder)
}

func codeDescription(code uint16) string {
	switch code {
	case 1:
		return "deposit"
	case 2:
		return "withdrawal"
	case 3:
		return "transfer"
	default:
		return "unknown"
	}
}

// unused: eliminar imports si no se usan
var _ = json.Marshal
