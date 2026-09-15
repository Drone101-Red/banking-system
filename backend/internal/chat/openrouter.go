package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"banking-system/internal/apperr"
)

const (
	// openRouterBaseURL es el endpoint de la API de OpenRouter.
	openRouterBaseURL = "https://openrouter.ai/api/v1"

	// defaultModel se usa si el cliente se construye sin modelo explícito.
	defaultModel = "openrouter/free"

	// requestTimeout es el timeout por request HTTP.
	requestTimeout = 60 * time.Second

	// maxRetries es el número de reintentos para errores transitorios.
	maxRetries = 3

	// retryBaseDelay es el delay inicial del backoff exponencial.
	retryBaseDelay = 1 * time.Second
)

// OpenRouterClient es un cliente HTTP para la API de OpenRouter.
//
// Usa el formato OpenAI de chat completions, que OpenRouter soporta.
type OpenRouterClient struct {
	apiKey  string
	model   string
	baseURL string
	http    *http.Client
}

// NewOpenRouterClient construye un cliente.
//
// Si model está vacío, usa defaultModel.
func NewOpenRouterClient(apiKey, model string) *OpenRouterClient {
	if model == "" {
		model = defaultModel
	}
	return &OpenRouterClient{
		apiKey:  apiKey,
		model:   model,
		baseURL: openRouterBaseURL,
		http: &http.Client{
			Timeout: requestTimeout,
		},
	}
}

// Model devuelve el modelo configurado.
func (c *OpenRouterClient) Model() string { return c.model }

// ChatCompletion envía una request de chat completion a OpenRouter.
//
// Reintenta hasta maxRetries veces para errores 5xx y 429 (rate limit).
// No reintenta para errores 4xx (excepto 429), que son problemas del request.
func (c *OpenRouterClient) ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if c.apiKey == "" {
		return nil, apperr.Internal(fmt.Errorf("OPENROUTER_API_KEY no configurada"))
	}
	if req.Model == "" {
		req.Model = c.model
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("marshal request: %w", err))
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Backoff exponencial: 1s, 2s, 4s
			delay := retryBaseDelay * time.Duration(1<<(attempt-1))
			select {
			case <-ctx.Done():
				return nil, apperr.Internal(ctx.Err())
			case <-time.After(delay):
			}
		}

		resp, err := c.doRequest(ctx, body)
		if err != nil {
			lastErr = err
			// Si es un error de red, reintentar.
			continue
		}

		// Errores 5xx o 429 → reintentar.
		if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))
			continue
		}

		// Errores 4xx (excepto 429) → no reintentar.
		if resp.StatusCode >= 400 {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, apperr.Internal(fmt.Errorf(
				"OpenRouter error HTTP %d: %s", resp.StatusCode, string(bodyBytes),
			))
		}

		// 2xx → parsear response.
		defer resp.Body.Close()
		var chatResp ChatResponse
		if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
			return nil, apperr.Internal(fmt.Errorf("decode response: %w", err))
		}

		if len(chatResp.Choices) == 0 {
			return nil, apperr.Internal(fmt.Errorf("OpenRouter devolvió 0 choices"))
		}

		return &chatResp, nil
	}

	return nil, apperr.Internal(fmt.Errorf(
		"OpenRouter no disponible después de %d intentos: %w", maxRetries+1, lastErr,
	))
}

// doRequest ejecuta la request HTTP.
func (c *OpenRouterClient) doRequest(ctx context.Context, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	// Headers recomendados por OpenRouter para analytics.
	req.Header.Set("HTTP-Referer", "http://localhost:8080")
	req.Header.Set("X-Title", "banking-system")

	return c.http.Do(req)
}

// --- Tipos del API (formato OpenAI, compatible con OpenRouter) ---

// ChatRequest es el cuerpo de POST /chat/completions.
type ChatRequest struct {
	Model      string           `json:"model"`
	Messages   []Message        `json:"messages"`
	Tools      []ToolDefinition `json:"tools,omitempty"`
	ToolChoice string           `json:"tool_choice,omitempty"`
}

// Message es un mensaje en la conversación.
type Message struct {
	Role       string     `json:"role"` // system, user, assistant, tool
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// ToolCall es una llamada a tool que hace el modelo.
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"` // "function"
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction contiene el nombre y argumentos de una tool call.
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // string JSON (formato OpenAI)
}

// ChatResponse es la respuesta de POST /chat/completions.
type ChatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Choice es una opción de respuesta.
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"` // stop, tool_calls, length
}

// Usage contiene el conteo de tokens.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
