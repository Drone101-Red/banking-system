package transactions

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"

	"banking-system/internal/apperr"
	"banking-system/internal/auth"
)

const idempotencyKeyHeader = "Idempotency-Key"

// Handler expone los endpoints HTTP de transacciones.
type Handler struct {
	service *Service
}

// NewHandler construye el Handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// DepositHandler maneja POST /api/transactions/deposit.
func (h *Handler) DepositHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apperr.Write(w, apperr.Unauthorized("TOKEN_REQUIRED", "Se requiere autenticación"))
		return
	}

	var req TransferRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		apperr.Write(w, apperr.BadRequest("INVALID_REQUEST", "El cuerpo de la solicitud no es válido"))
		return
	}

	idemKey := r.Header.Get(idempotencyKeyHeader)

	tx, err := h.service.Deposit(r.Context(), userID, req.AmountCents, idemKey)
	if err != nil {
		apperr.Write(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, tx)
}

// WithdrawHandler maneja POST /api/transactions/withdraw.
func (h *Handler) WithdrawHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apperr.Write(w, apperr.Unauthorized("TOKEN_REQUIRED", "Se requiere autenticación"))
		return
	}

	var req TransferRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		apperr.Write(w, apperr.BadRequest("INVALID_REQUEST", "El cuerpo de la solicitud no es válido"))
		return
	}

	idemKey := r.Header.Get(idempotencyKeyHeader)

	tx, err := h.service.Withdraw(r.Context(), userID, req.AmountCents, idemKey)
	if err != nil {
		apperr.Write(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, tx)
}

// TransferHandler maneja POST /api/transactions/transfer.
func (h *Handler) TransferHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apperr.Write(w, apperr.Unauthorized("TOKEN_REQUIRED", "Se requiere autenticación"))
		return
	}

	var req UserTransferRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		apperr.Write(w, apperr.BadRequest("INVALID_REQUEST", "El cuerpo de la solicitud no es válido"))
		return
	}

	idemKey := r.Header.Get(idempotencyKeyHeader)

	tx, err := h.service.Transfer(r.Context(), userID, req.ToAccountID, req.AmountCents, idemKey)
	if err != nil {
		apperr.Write(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, tx)
}

// HistoryHandler maneja GET /api/transactions/history.
func (h *Handler) HistoryHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apperr.Write(w, apperr.Unauthorized("TOKEN_REQUIRED", "Se requiere autenticación"))
		return
	}

	page := parseIntQuery(r, "page", 1)
	limit := parseIntQuery(r, "limit", 20)

	result, err := h.service.History(r.Context(), userID, page, limit)
	if err != nil {
		apperr.Write(w, err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// DemoTopupHandler maneja POST /api/transactions/demo-topup.
//
// Solo disponible en APP_ENV=development.
func (h *Handler) DemoTopupHandler(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("APP_ENV") != "development" {
		apperr.Write(w, apperr.Forbidden(
			"DEMO_DISABLED",
			"El crédito de demo solo está disponible en desarrollo",
		))
		return
	}

	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apperr.Write(w, apperr.Unauthorized("TOKEN_REQUIRED", "Se requiere autenticación"))
		return
	}

	result, err := h.service.DemoTopup(r.Context(), userID)
	if err != nil {
		apperr.Write(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[transactions] error serializando respuesta: %v", err)
	}
}

func parseIntQuery(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
