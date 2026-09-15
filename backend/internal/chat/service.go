// Package chat implementa el chat con IA sobre OpenRouter.
package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

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
	// Cada turno son 2 mensajes (user + assistant), así que 20
	// mensajes = 10 turnos de conversación.
	maxHistoryMessages = 20
)

// SystemPrompt es el system prompt del asistente.
//
// Es crítico: define las reglas de comportamiento, en particular
// la obligación de pedir confirmación antes de ejecutar tools de
// escritura.
const SystemPrompt = `Eres un asistente bancario que ayuda al usuario a gestionar su cuenta.

REGLAS:
- Responde SIEMPRE en español, de forma clara y concisa.
- Los montos están en CENTAVOS. $10.50 = 1050 centavos.
- SIEMPRE pide confirmación explícita antes de ejecutar deposit, withdraw o transfer.
  Ejemplo: "Voy a transferir $50.00 a la cuenta abc... ¿Confirmas?"
- Solo ejecuta la tool cuando el usuario confirme ("sí", "confirmo", "dale", etc.).
- Si una tool falla, explica el error al usuario sin tecnicismos.
- No inventes datos. Usa las tools para consultar información real.
- No reveles detalles técnicos (nombres de tablas, IDs internos, etc.).
- No ejecutes más de una operación de escritura por turno.`

// ChatRequestDTO es el body de POST /api/chat.
type ChatRequestDTO struct {
	Message string    `json:"message"`
	History []Message `json:"history,omitempty"`
}

// ChatResult es la respuesta del chat.
type ChatResult struct {
	Reply     string     `json:"reply"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Usage     Usage      `json:"usage"`
}

// Service orquesta el chat con tool calling.
type Service struct {
	pg         *db.PostgresStore
	accountSvc *account.Service
	txnSvc     *transactions.Service
	client     *OpenRouterClient
	executor   *Executor
}

// NewService construye el Service.
func NewService(
	pg *db.PostgresStore,
	accountSvc *account.Service,
	txnSvc *transactions.Service,
	client *OpenRouterClient,
) *Service {
	return &Service{
		pg:         pg,
		accountSvc: accountSvc,
		txnSvc:     txnSvc,
		client:     client,
		executor:   NewExecutor(accountSvc, txnSvc),
	}
}

// Chat procesa un mensaje del usuario y devuelve la respuesta del modelo.
//
// El userID viene del JWT (middleware RequireAuth). El modelo NUNCA
// lo ve. Cuando ejecuta una tool, el Executor lo usa para operar sobre
// la cuenta del usuario autenticado.
//
// Loop:
//  1. Construir messages (system + history + user message).
//  2. Llamar a OpenRouter con las tools.
//  3. Si el modelo pide tools: ejecutarlas, agregar resultados, repetir.
//  4. Si el modelo da respuesta final: devolverla.
//
// Casos manejados:
//   - No hay tool calls → respuesta final.
//   - Hay tool calls y content no vacío → ejecutar tools y devolver content.
//   - Hay tool calls sin content → ejecutar tools y seguir el loop.
//
// Si el modelo se queda en loop o no da respuesta, se devuelve un
// fallback construido a partir de las tools ejecutadas.
func (s *Service) Chat(
	ctx context.Context,
	userID string,
	req ChatRequestDTO,
) (*ChatResult, error) {
	if strings.TrimSpace(req.Message) == "" {
		return nil, apperr.BadRequest("EMPTY_MESSAGE", "El mensaje no puede estar vacío")
	}

	messages := s.buildInitialMessages(req)

	var executedTools []ToolCall
	var totalUsage Usage

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

		// --- Caso 1: no hay tool calls → respuesta final. ---
		if len(choice.Message.ToolCalls) == 0 {
			reply := strings.TrimSpace(choice.Message.Content)
			if reply == "" {
				reply = buildFallbackReply(executedTools)
			}
			return &ChatResult{
				Reply:     reply,
				ToolCalls: executedTools,
				Usage:     totalUsage,
			}, nil
		}

		// --- Caso 2: hay tool calls Y content no vacío. ---
		// El modelo quiere ejecutar tools Y decir algo.
		// Ejecutamos las tools y devolvemos el content como respuesta.
		if strings.TrimSpace(choice.Message.Content) != "" {
			for _, tc := range choice.Message.ToolCalls {
				if _, err := s.executeToolCall(ctx, userID, tc); err != nil {
					log.Printf("[chat] tool %s falló: %v", tc.Function.Name, err)
				}
				executedTools = append(executedTools, tc)
			}
			return &ChatResult{
				Reply:     strings.TrimSpace(choice.Message.Content),
				ToolCalls: executedTools,
				Usage:     totalUsage,
			}, nil
		}

		// --- Caso 3: hay tool calls sin content → ejecutar y seguir. ---
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

	// Si llegamos acá, el modelo no dio respuesta final en N iteraciones.
	// Devolvemos un fallback con las tools que sí se ejecutaron.
	return &ChatResult{
		Reply:     buildFallbackReply(executedTools),
		ToolCalls: executedTools,
		Usage:     totalUsage,
	}, nil
}

// buildInitialMessages construye el array de mensajes inicial.
func (s *Service) buildInitialMessages(req ChatRequestDTO) []Message {
	messages := []Message{
		{Role: "system", Content: SystemPrompt},
	}

	// Agregar historial (limitado)
	history := req.History
	if len(history) > maxHistoryMessages {
		history = history[len(history)-maxHistoryMessages:]
	}
	messages = append(messages, history...)

	// Agregar mensaje nuevo del usuario
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

	// Parsear argumentos (JSON string).
	var args map[string]any
	if tc.Function.Arguments != "" {
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			return nil, fmt.Errorf("parsear argumentos de %s: %w", tc.Function.Name, err)
		}
	}
	if args == nil {
		args = map[string]any{}
	}

	return s.executor.Execute(ctx, userID, tc.Function.Name, args)
}

// buildFallbackReply genera una respuesta si el modelo no dio una.
//
// Se usa cuando:
//   - El modelo devolvió content vacío en la respuesta final.
//   - El modelo se quedó en loop sin dar respuesta final.
//
// En ambos casos, construimos un mensaje a partir de las tools que
// sí se ejecutaron.
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
		return "Depósito realizado correctamente."
	case "withdraw":
		return "Retiro realizado correctamente."
	case "transfer":
		return "Transferencia realizada correctamente."
	default:
		return "Operación completada."
	}
}
