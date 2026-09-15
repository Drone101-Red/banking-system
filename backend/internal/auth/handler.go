package auth

import (
	"encoding/json"
	"log"
	"net/http"

	"banking-system/internal/apperr"
)

// Handler expone los endpoints HTTP de autenticación.
type Handler struct {
	service *Service
}

// NewHandler construye el Handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterHandler maneja POST /api/auth/register.
func (h *Handler) RegisterHandler(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(&req); err != nil {
		apperr.Write(w, apperr.BadRequest(
			"INVALID_REQUEST",
			"El cuerpo de la solicitud no es válido",
		))
		return
	}

	user, err := h.service.Register(r.Context(), req)
	if err != nil {
		apperr.Write(w, err)
		return
	}

	// Respuesta: solo campos públicos. PasswordHash y TBAccountID
	// tienen json:"-" en el modelo, así que no se filtran aunque
	// serialicemos el user completo.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(map[string]any{
		"id":        user.ID,
		"email":     user.Email,
		"full_name": user.FullName,
		"status":    user.Status,
	}); err != nil {
		log.Printf("[auth] error serializando respuesta: %v", err)
	}
}
