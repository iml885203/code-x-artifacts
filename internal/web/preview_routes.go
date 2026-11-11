package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

type previewIdentity struct {
	Key     string `json:"key"`
	Version int    `json:"version"`
	URL     string `json:"url,omitempty"`
	expires time.Time
}

// previewIdentity revalidates the current account's access before consulting
// cached bytes. Asset mutations invalidate independently of saved versions.
func (s *Server) previewIdentity(w http.ResponseWriter, r *http.Request) (previewIdentity, bool) {
	width, errW := strconv.Atoi(r.URL.Query().Get("w"))
	height, errH := strconv.Atoi(r.URL.Query().Get("h"))
	if errW != nil || errH != nil || width < 240 || width > 2000 || height < 120 || height > 2000 {
		jsonError(w, http.StatusBadRequest, "preview dimensions must be width 240..2000 and height 120..2000")
		return previewIdentity{}, false
	}
	context, err := previewRenderContext(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return previewIdentity{}, false
	}
	a, err := s.svc.Artifact(r.Context(), currentViewer(r), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return previewIdentity{}, false
	}
	assets, err := s.svc.Assets(r.Context(), currentViewer(r), a.ID)
	if err != nil {
		s.fail(w, err)
		return previewIdentity{}, false
	}
	now := s.previews.now()
	key := previewKey(currentUser(r), a, assets, width, height, context, now)
	return previewIdentity{Key: key, Version: a.Version, expires: now.Truncate(time.Hour).Add(time.Hour)}, true
}

var previewLanguage = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

// Omitted hints retain the legacy/unspecified context. Explicit browser hints
// prevent a ready DOM captured under one environment from replacing another.
func previewRenderContext(r *http.Request) (string, error) {
	query := r.URL.Query()
	scheme, motion, lang := query.Get("scheme"), query.Get("motion"), query.Get("lang")
	if query.Has("scheme") && scheme != "light" && scheme != "dark" {
		return "", fmt.Errorf("preview scheme must be light or dark")
	}
	if query.Has("motion") && motion != "reduce" && motion != "no-preference" {
		return "", fmt.Errorf("preview motion must be reduce or no-preference")
	}
	if query.Has("lang") && (len(lang) > 64 || !previewLanguage.MatchString(lang)) {
		return "", fmt.Errorf("preview language is invalid")
	}
	dpr := "1"
	if query.Has("dpr") {
		value, err := strconv.ParseFloat(query.Get("dpr"), 64)
		if err != nil || !(value >= 0.5 && value <= 4) {
			return "", fmt.Errorf("preview pixel ratio must be between 0.5 and 4")
		}
		dpr = strconv.FormatFloat(value, 'f', -1, 64)
	}
	return baseURL(r) + "\x00" + scheme + "\x00" + motion + "\x00" + lang + "\x00" + dpr, nil
}

func previewKey(owner string, a store.Artifact, assets []store.Asset, width, height int, context string, now time.Time) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "preview-dom-v1\x00%s\x00%s\x00%d\x00%s\x00%s\x00%s\x00%d\x00%d\x00%d", owner, a.ID, a.Version, a.Kind, a.Format, a.Visibility, width, height, now.Unix()/3600)
	_, _ = fmt.Fprintf(h, "\x00%s", context)
	for _, asset := range assets {
		_, _ = fmt.Fprintf(h, "\x00%s\x00%s\x00%s", asset.Name, asset.SHA256, asset.ContentType)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (s *Server) previewMetadata(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	identity, ok := s.previewIdentity(w, r)
	if !ok {
		return
	}
	if _, found := s.previews.get(identity.Key); found {
		query := r.URL.Query()
		query.Set("key", identity.Key)
		identity.URL = "/preview/" + r.PathValue("id") + "?" + query.Encode()
	}
	writeJSON(w, http.StatusOK, identity)
}

func (s *Server) savePreview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	identity, ok := s.previewIdentity(w, r)
	if !ok {
		return
	}
	var in struct {
		Key  string `json:"key"`
		HTML string `json:"html"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPreviewBytes+(1<<16)))
	if err := decoder.Decode(&in); err != nil {
		jsonError(w, http.StatusBadRequest, "send a bounded preview snapshot")
		return
	}
	if in.Key != identity.Key {
		jsonError(w, http.StatusConflict, "preview changed")
		return
	}
	if !s.previews.put(identity.Key, in.HTML, identity.expires) {
		jsonError(w, http.StatusRequestEntityTooLarge, "preview snapshot is empty or too large")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) cachedPreview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	identity, ok := s.previewIdentity(w, r)
	if !ok {
		return
	}
	if r.URL.Query().Get("key") != identity.Key {
		http.Redirect(w, r, "/raw/"+r.PathValue("id")+"?preview", http.StatusFound)
		return
	}
	html, found := s.previews.get(identity.Key)
	if !found {
		http.Redirect(w, r, "/raw/"+r.PathValue("id")+"?preview", http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "sandbox; script-src 'none'; object-src 'none'; connect-src 'none'; form-action 'none'; frame-ancestors 'self'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Robots-Tag", "noindex")
	_, _ = io.WriteString(w, html)
}

func (s *Server) routesPreviews(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/artifacts/{id}/preview", s.browserAPI(s.previewMetadata))
	mux.HandleFunc("POST /api/artifacts/{id}/preview", s.browserAPI(s.savePreview))
	// An iframe navigation cannot send X-Pictor. A live browser session is
	// required; absent/revoked sessions receive401, never a login document.
	mux.HandleFunc("GET /preview/{id}", func(w http.ResponseWriter, r *http.Request) {
		user, ok := s.sessionUser(r)
		if !ok {
			w.Header().Set("Cache-Control", "no-store")
			jsonError(w, http.StatusUnauthorized, "sign in")
			return
		}
		s.cachedPreview(w, s.signedIn(r, user))
	})
}
