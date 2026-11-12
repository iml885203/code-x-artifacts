package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image/jpeg"
	"io"
	"net/http"
	"net/url"
	"time"
)

type thumbnailIdentity struct {
	Key    string `json:"key"`
	URL    string `json:"url,omitempty"`
	Source string `json:"source,omitempty"`
	group  string
}

func (s *Server) thumbnailIdentity(w http.ResponseWriter, r *http.Request) (thumbnailIdentity, bool) {
	q := r.URL.Query()
	scheme, lang := q.Get("scheme"), q.Get("lang")
	if (scheme != "light" && scheme != "dark") || (lang != "en" && lang != "zh-TW") {
		jsonError(w, 400, "invalid thumbnail variant")
		return thumbnailIdentity{}, false
	}
	viewport := q.Get("viewport")
	if viewport == "" {
		viewport = "legacy"
	}
	if viewport != "legacy" && viewport != "mobile" && viewport != "tablet" && viewport != "desktop" {
		jsonError(w, 400, "invalid thumbnail viewport")
		return thumbnailIdentity{}, false
	}
	a, err := s.svc.Artifact(r.Context(), currentViewer(r), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return thumbnailIdentity{}, false
	}
	assets, err := s.svc.Assets(r.Context(), currentViewer(r), a.ID)
	if err != nil {
		s.fail(w, err)
		return thumbnailIdentity{}, false
	}
	context := "thumbnail-jpeg-v2\x00" + viewport + "\x00" + scriptHash(s.thumbnailScript) + "\x00" + s.bundle.Entry + "\x00" + baseURL(r) + "\x00" + scheme + "\x00" + lang
	key := previewKey(currentUser(r), a, assets, thumbnailWidth, thumbnailHeight, context, time.Unix(0, 0))
	sum := sha256.Sum256([]byte(currentUser(r) + "\x00" + a.ID + "\x00" + scheme + "\x00" + lang + "\x00" + viewport))
	group := hex.EncodeToString(sum[:])
	return thumbnailIdentity{Key: key, group: group}, true
}
func (t thumbnailIdentity) name() string { return t.group + "-" + t.Key + ".jpg" }
func (s *Server) thumbnailMetadata(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	c := s.thumbnails
	if c == nil || c.dir == "" {
		jsonError(w, 503, "thumbnail storage unavailable")
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	identity, ok := s.thumbnailIdentity(w, r)
	if !ok {
		return
	}
	if c.has(identity.name()) {
		q := r.URL.Query()
		q.Set("key", identity.Key)
		identity.URL = "/thumbnail/" + r.PathValue("id") + "?" + q.Encode()
	} else {
		identity.Source = "/raw/" + url.PathEscape(r.PathValue("id")) + "?preview&" + r.URL.RawQuery
	}
	writeJSON(w, 200, identity)
}

func (s *Server) saveThumbnail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	c := s.thumbnails
	if c == nil || c.dir == "" {
		jsonError(w, 503, "thumbnail storage unavailable")
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxThumbnailBytes))
	if err != nil {
		jsonError(w, 413, "thumbnail too large")
		return
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width != thumbnailWidth || config.Height != thumbnailHeight {
		jsonError(w, 400, "send a 1120 by 720 JPEG thumbnail")
		return
	}
	image, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		jsonError(w, 400, "invalid thumbnail")
		return
	}
	var clean bytes.Buffer
	if err = jpeg.Encode(&clean, image, &jpeg.Options{Quality: 82}); err != nil {
		s.serverError(w, err)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	identity, ok := s.thumbnailIdentity(w, r)
	if !ok {
		return
	}
	if r.URL.Query().Get("key") != identity.Key {
		jsonError(w, 409, "thumbnail changed")
		return
	}
	if err = c.put(identity.name(), clean.Bytes()); err != nil {
		s.log.Warn("cache thumbnail", "err", err)
		jsonError(w, 503, "thumbnail storage unavailable")
		return
	}
	w.WriteHeader(204)
}

func (s *Server) serveThumbnail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	c := s.thumbnails
	if c == nil {
		jsonError(w, 404, "thumbnail missing")
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	identity, ok := s.thumbnailIdentity(w, r)
	if !ok {
		return
	}
	if r.URL.Query().Get("key") != identity.Key {
		jsonError(w, 404, "thumbnail changed")
		return
	}
	data, ok := c.get(identity.name())
	if !ok {
		jsonError(w, 404, "thumbnail missing")
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	_, _ = w.Write(data)
}

func (s *Server) routesThumbnails(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/artifacts/{id}/thumbnail", s.browserAPI(s.thumbnailMetadata))
	mux.HandleFunc("POST /api/artifacts/{id}/thumbnail", s.browserAPI(s.saveThumbnail))
	mux.HandleFunc("GET /thumbnail/{id}", func(w http.ResponseWriter, r *http.Request) {
		user, ok := s.sessionUser(r)
		if !ok {
			w.Header().Set("Cache-Control", "no-store")
			jsonError(w, 401, "sign in")
			return
		}
		s.serveThumbnail(w, s.signedIn(r, user))
	})
}
