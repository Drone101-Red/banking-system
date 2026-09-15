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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(map[string]any{
		"id":        user.ID,
		"email":     user.Email,
		"full_name": user.FullName,
		"alias":     user.Alias,
		"status":    user.Status,
	}); err != nil {
		log.Printf("[auth] error serializando respuesta register: %v", err)
	}
}

// LoginHandler maneja POST /api/auth/login.
func (h *Handler) LoginHandler(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(&req); err != nil {
		apperr.Write(w, apperr.BadRequest(
			"INVALID_REQUEST",
			"El cuerpo de la solicitud no es válido",
		))
		return
	}

	user, token, err := h.service.Login(r.Context(), req)
	if err != nil {
		apperr.Write(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]any{
		"token": token,
		"user": map[string]any{
			"id":        user.ID,
			"email":     user.Email,
			"full_name": user.FullName,
			"alias":     user.Alias,
			"status":    user.Status,
		},
	}); err != nil {
		log.Printf("[auth] error serializando respuesta login: %v", err)
	}
}

// LogoutHandler maneja POST /api/auth/logout.
func (h *Handler) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// MeHandler maneja GET /api/auth/me.
func (h *Handler) MeHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		apperr.Write(w, apperr.Unauthorized(
			"TOKEN_REQUIRED",
			"Se requiere autenticación",
		))
		return
	}

	user, err := h.service.pg.GetUserByID(r.Context(), userID)
	if err != nil {
		apperr.Write(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]any{
		"id":        user.ID,
		"email":     user.Email,
		"full_name": user.FullName,
		"alias":     user.Alias,
		"status":    user.Status,
	}); err != nil {
		log.Printf("[auth] error serializando respuesta me: %v", err)
	}
}

// LookupHandler maneja GET /api/users/lookup.
//
// Query params:
//   - email: buscar por email
//   - alias: buscar por alias
//
// Exactamente uno de los dos debe estar presente.
func (h *Handler) LookupHandler(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	alias := r.URL.Query().Get("alias")

	result, err := h.service.LookupUser(r.Context(), email, alias)
	if err != nil {
		apperr.Write(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(result); err != nil {
		log.Printf("[auth] error serializando respuesta lookup: %v", err)
	}
}
