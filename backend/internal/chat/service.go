package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"banking-system/internal/account"
	"banking-system/internal/apperr"
	"banking-system/internal/db"
	"banking-system/internal/transactions"
)

const (
	// maxToolCallIterations acota el número de vueltas del loop
	// para evitar loops infinitos si el modelo no coopera.
	maxToolCallIterations = 10

	// maxHistoryMessages acota el historial que se envía al modelo.
	maxHistoryMessages = 20
)

// SystemPrompt es el system prompt del asistente.
//
// Es crítico: define las reglas de comportamiento, en particular
// la obligación de llamar a la tool INMEDIATAMENTE para operaciones
// de escritura. El backend se encarga de la confirmación mediante
// un token de dos fases.
const SystemPrompt = `Eres un asistente bancario que ayuda al usuario a gestionar su cuenta.

REGLAS:
- Responde SIEMPRE en español, de forma clara y concisa.
- Los montos están en CENTAVOS. $10.50 = 1050 centavos.
- No inventes datos. Usa las tools para consultar información real.
- No reveles detalles técnicos (nombres de tablas, IDs internos, etc.).
- No ejecutes más de una operación de escritura por turno.

OPERACIONES DE ESCRITURA (deposit, withdraw, transfer):
Cuando el usuario pida una operación de escritura, LLAMA A LA TOOL
INMEDIATAMENTE, sin pedir confirmación previa. Ejemplo:

  Usuario: "Deposita $50 a mi cuenta"
  Tú: [llamas a la tool deposit con amount_cents=5000]

La tool NO ejecutará la operación. Devolverá un resultado con
status="pending_confirmation" y un token. En ese momento:
  1. Informa al usuario qué operación está pendiente.
  2. Pide confirmación explícita ("¿Confirmas?").
  3. El frontend llamará a POST /api/chat/confirm con el token.
  4. Tú NO necesitas llamar a ninguna tool adicional.

NUNCA pidas confirmación ANTES de llamar a la tool.
La confirmación ocurre DESPUÉS, cuando el usuario ve el token.`

// ChatRequestDTO es el body de POST /api/chat.
type ChatRequestDTO struct {
	Message string    `json:"message"`
	History []Message `json:"history,omitempty"`
}

// ChatResult es la respuesta del chat.
type ChatResult struct {
	Reply               string            `json:"reply"`
	ToolCalls           []ToolCall        `json:"tool_calls,omitempty"`
	PendingConfirmation *PendingOperation `json:"pending_confirmation,omitempty"`
	Usage               Usage             `json:"usage"`
}

// ConfirmResult es la respuesta de POST /api/chat/confirm.
type ConfirmResult struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	Operation string `json:"operation"`
}

// Service orquesta el chat con tool calling.
type Service struct {
	pg           *db.PostgresStore
	accountSvc   *account.Service
	txnSvc       *transactions.Service
	client       *OpenRouterClient
	executor     *Executor
	confirmStore ConfirmationStore
}

// NewService construye el Service.
func NewService(
	pg *db.PostgresStore,
	accountSvc *account.Service,
	txnSvc *transactions.Service,
	client *OpenRouterClient,
	confirmStore ConfirmationStore,
) *Service {
	return &Service{
		pg:           pg,
		accountSvc:   accountSvc,
		txnSvc:       txnSvc,
		client:       client,
		confirmStore: confirmStore,
		executor:     NewExecutor(accountSvc, txnSvc, confirmStore),
	}
}

// Chat procesa un mensaje del usuario y devuelve la respuesta del modelo.
//
// El userID viene del JWT (middleware RequireAuth). El modelo NUNCA
// lo ve. Cuando ejecuta una tool, el Executor lo usa para operar sobre
// la cuenta del usuario autenticado.
func (s *Service) Chat(
	ctx context.Context,
	userID string,
	req ChatRequestDTO,
) (*ChatResult, error) {
	if strings.TrimSpace(req.Message) == "" {
		return nil, apperr.BadRequest("EMPTY_MESSAGE", "El mensaje no puede estar vacío")
	}

	messages := s.buildInitialMessages(req)

	var (
		executedTools  []ToolCall
		totalUsage     Usage
		pendingConfirm *PendingOperation
	)

	for i := 0; i < maxToolCallIterations; i++ {
		resp, err := s.client.ChatCompletion(ctx, ChatRequest{
			Messages: messages,
			Tools:    Tools(),
		})
		if err != nil {
			return nil, err
		}

		totalUsage.PromptTokens += resp.Usage.PromptTokens
		totalUsage.CompletionTokens += resp.Usage.CompletionTokens
		totalUsage.TotalTokens += resp.Usage.TotalTokens

		choice := resp.Choices[0]

		// Caso 1: no hay tool calls → respuesta final.
		if len(choice.Message.ToolCalls) == 0 {
			reply := choice.Message.Content
			if strings.TrimSpace(reply) == "" {
				reply = buildFallbackReply(executedTools)
			}
			return &ChatResult{
				Reply:               reply,
				ToolCalls:           executedTools,
				PendingConfirmation: pendingConfirm,
				Usage:               totalUsage,
			}, nil
		}

		// Caso 2: hay tool calls + content no vacío.
		// El modelo quiere ejecutar tools Y decir algo.
		if strings.TrimSpace(choice.Message.Content) != "" {
			for _, tc := range choice.Message.ToolCalls {
				result, err := s.executeToolCall(ctx, userID, tc)
				if err == nil {
					if op := extractPendingConfirmation(result, userID); op != nil {
						pendingConfirm = op
					}
				}
				executedTools = append(executedTools, tc)
			}
			return &ChatResult{
				Reply:               choice.Message.Content,
				ToolCalls:           executedTools,
				PendingConfirmation: pendingConfirm,
				Usage:               totalUsage,
			}, nil
		}

		// Caso 3: hay tool calls sin content → ejecutar y seguir el loop.
		messages = append(messages, choice.Message)

		for _, tc := range choice.Message.ToolCalls {
			result, err := s.executeToolCall(ctx, userID, tc)

			var content string
			if err != nil {
				log.Printf("[chat] tool %s falló: %v", tc.Function.Name, err)
				content = fmt.Sprintf(`{"error": %q}`, err.Error())
			} else {
				contentBytes, _ := json.Marshal(result)
				content = string(contentBytes)

				if op := extractPendingConfirmation(result, userID); op != nil {
					pendingConfirm = op
				}
			}

			messages = append(messages, Message{
				Role:       "tool",
				ToolCallID: tc.ID,
				Name:       tc.Function.Name,
				Content:    content,
			})

			executedTools = append(executedTools, tc)
		}
	}

	return nil, apperr.Internal(fmt.Errorf(
		"el modelo no dio respuesta final después de %d iteraciones",
		maxToolCallIterations,
	))
}

// ConfirmOperation ejecuta la operación pendiente identificada por el token.
//
// Es el segundo paso del two-phase commit:
//  1. El modelo propone la operación → se guarda como pending.
//  2. El usuario confirma → se ejecuta.
func (s *Service) ConfirmOperation(
	ctx context.Context,
	userID, token string,
) (*ConfirmResult, error) {
	op, err := s.confirmStore.Consume(ctx, token, userID)
	if err != nil {
		return nil, err
	}

	switch op.Type {
	case "deposit":
		_, err = s.txnSvc.Deposit(ctx, userID, op.AmountCents, token)
	case "withdraw":
		_, err = s.txnSvc.Withdraw(ctx, userID, op.AmountCents, token)
	case "transfer":
		_, err = s.txnSvc.Transfer(ctx, userID, op.ToAccountID, op.AmountCents, token)
	default:
		return nil, apperr.Internal(fmt.Errorf("tipo de operación desconocido: %s", op.Type))
	}

	if err != nil {
		return nil, err
	}

	return &ConfirmResult{
		Success:   true,
		Message:   fmt.Sprintf("Operación '%s' ejecutada correctamente", op.Type),
		Operation: op.Type,
	}, nil
}

// buildInitialMessages construye el array de mensajes inicial.
func (s *Service) buildInitialMessages(req ChatRequestDTO) []Message {
	messages := []Message{
		{Role: "system", Content: SystemPrompt},
	}

	history := req.History
	if len(history) > maxHistoryMessages {
		history = history[len(history)-maxHistoryMessages:]
	}
	messages = append(messages, history...)

	messages = append(messages, Message{
		Role:    "user",
		Content: req.Message,
	})

	return messages
}

// executeToolCall ejecuta una tool call del modelo.
func (s *Service) executeToolCall(
	ctx context.Context,
	userID string,
	tc ToolCall,
) (any, error) {
	if tc.Type != "function" {
		return nil, fmt.Errorf("tipo de tool no soportado: %s", tc.Type)
	}

	var args map[string]any
	if tc.Function.Arguments != "" {
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			return nil, fmt.Errorf("parsear argumentos de %s: %w", tc.Function.Name, err)
		}
	}
	if args == nil {
		args = map[string]any{}
	}

	return s.executor.Execute(ctx, userID, tc.ID, tc.Function.Name, args)
}

// --- Helpers ---

// buildFallbackReply genera una respuesta si el modelo no dio una.
func buildFallbackReply(tools []ToolCall) string {
	if len(tools) == 0 {
		return "No pude procesar tu mensaje. ¿Podrías reformularlo?"
	}
	last := tools[len(tools)-1].Function.Name
	switch last {
	case "get_balance":
		return "Consulté tu saldo."
	case "get_history":
		return "Consulté tu historial."
	case "deposit":
		return "Depósito pendiente de confirmación."
	case "withdraw":
		return "Retiro pendiente de confirmación."
	case "transfer":
		return "Transferencia pendiente de confirmación."
	default:
		return "Operación completada."
	}
}

// getString devuelve un valor string de un map, o "" si no existe.
func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// extractPendingConfirmation extrae una PendingOperation del resultado
// de una tool, si el resultado contiene status="pending_confirmation".
//
// Es defensivo con los tipos: el resultado puede venir de:
//   - map[string]any con int64 (creado en Go).
//   - map[string]any con float64 (deserializado de JSON).
//
// Devuelve nil si el resultado no es una pending confirmation.
func extractPendingConfirmation(result any, userID string) *PendingOperation {
	op, ok := result.(map[string]any)
	if !ok {
		return nil
	}

	status, ok := op["status"].(string)
	if !ok || status != "pending_confirmation" {
		return nil
	}

	token, ok := op["confirmation_token"].(string)
	if !ok || token == "" {
		return nil
	}

	opType, ok := op["type"].(string)
	if !ok {
		return nil
	}

	pending := &PendingOperation{
		Token:       token,
		Type:        opType,
		AmountCents: extractInt64(op["amount_cents"]),
		ToAccountID: getString(op, "to_account_id"),
		UserID:      userID,
	}

	// expires_at puede venir como time.Time (Executor) o string (JSON).
	switch v := op["expires_at"].(type) {
	case time.Time:
		pending.ExpiresAt = v
	case string:
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			pending.ExpiresAt = t
		}
	}

	return pending
} // extractInt64 convierte un valor de map a int64.
// Acepta int64, float64, int y json.Number. Devuelve 0 si no puede.
// Evita el panic de type assertions directas.
func extractInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	case int:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	default:
		return 0
	}
}
