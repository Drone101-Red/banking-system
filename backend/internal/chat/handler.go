package chat

import (
	"encoding/json"
	"log"
	"net/http"

	"banking-system/internal/apperr"
	"banking-system/internal/auth"
)

// Handler expone los endpoints HTTP del chat.
type Handler struct {
	service *Service
}

// NewHandler construye el Handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// ChatHandler maneja POST /api/chat.
//
// Requiere el middleware RequireAuth corriendo antes.
func (h *Handler) ChatHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apperr.Write(w, apperr.Unauthorized("TOKEN_REQUIRED", "Se requiere autenticación"))
		return
	}

	var req ChatRequestDTO
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		apperr.Write(w, apperr.BadRequest("INVALID_REQUEST", "El cuerpo de la solicitud no es válido"))
		return
	}

	result, err := h.service.Chat(r.Context(), userID, req)
	if err != nil {
		apperr.Write(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(result); err != nil {
		log.Printf("[chat] error serializando respuesta: %v", err)
	}
}
