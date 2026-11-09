package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"
)

// maxTokenNameChars is the longest name a token may be given.
const maxTokenNameChars = 60

// listTokensJSON is the person's own tokens, newest first, without secrets.
func (s *Server) listTokensJSON(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.Tokens(r.Context(), currentUser(r))
	if err != nil {
		s.serverError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, list)
}

// createTokenJSON makes a token for the connect page, which shows it once and
// keeps it only in memory. The secret is in this response and nowhere else.
func (s *Server) createTokenJSON(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&in); err != nil {
		jsonError(w, http.StatusBadRequest, "send {\"name\": \"...\"}")
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = "token"
	}
	if utf8.RuneCountInString(name) > maxTokenNameChars {
		jsonError(w, http.StatusBadRequest, "the name is too long")
		return
	}
	plain, err := s.store.CreateToken(r.Context(), currentUser(r), name)
	if err != nil {
		s.serverError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]string{"token": plain, "name": name})
}

// deleteTokenJSON revokes one of the person's own tokens at once: the next
// request that carries it is refused.
func (s *Server) deleteTokenJSON(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteToken(r.Context(), currentUser(r), r.PathValue("id")); err != nil {
		s.notFoundOr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// routesTokens registers personal tokens.
func (s *Server) routesTokens(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/tokens", s.browserAPI(s.listTokensJSON))
	mux.HandleFunc("POST /api/tokens", s.browserAPI(s.createTokenJSON))
	mux.HandleFunc("DELETE /api/tokens/{id}", s.browserAPI(s.deleteTokenJSON))
}
