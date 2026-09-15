package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLogoutHandler_Returns204(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	rec := httptest.NewRecorder()

	h.LogoutHandler(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, esperado 204", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("body debería estar vacío, obtuve %q", rec.Body.String())
	}
}

func TestLogoutHandler_IgnoresAuthorization(t *testing.T) {
	// El logout no requiere JWT. Con o sin header, debe devolver 204.
	h := &Handler{}

	cases := []struct {
		name   string
		header string
	}{
		{"sin header", ""},
		{"con bearer valido", "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.xxx.yyy"},
		{"con bearer invalido", "Bearer no.es.un.jwt"},
		{"con prefijo raro", "Token abc"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
			if c.header != "" {
				req.Header.Set("Authorization", c.header)
			}
			rec := httptest.NewRecorder()

			h.LogoutHandler(rec, req)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d, esperado 204", rec.Code)
			}
		})
	}
}
