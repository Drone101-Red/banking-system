package account

import (
	"encoding/json"
	"log"
	"net/http"

	"banking-system/internal/apperr"
	"banking-system/internal/auth"
)

// Handler expone los endpoints HTTP de cuenta.
type Handler struct {
	service *Service
}

// NewHandler construye el Handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// InfoHandler maneja GET /api/account.
func (h *Handler) InfoHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apperr.Write(w, apperr.Unauthorized("TOKEN_REQUIRED", "Se requiere autenticación"))
		return
	}

	info, err := h.service.Info(r.Context(), userID)
	if err != nil {
		apperr.Write(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(info); err != nil {
		log.Printf("[account] error serializando info: %v", err)
	}
}

// BalanceHandler maneja GET /api/account/balance.
func (h *Handler) BalanceHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apperr.Write(w, apperr.Unauthorized("TOKEN_REQUIRED", "Se requiere autenticación"))
		return
	}

	balance, err := h.service.Balance(r.Context(), userID)
	if err != nil {
		apperr.Write(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]int64{
		"balance_cents": balance,
	}); err != nil {
		log.Printf("[account] error serializando balance: %v", err)
	}
}
