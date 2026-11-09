package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/files"
	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/mcptools"
	"github.com/iml885203/code-x-artifacts/internal/service"
)

// exportArtifact downloads the artifact in a format its kind offers, as the
// version the query names (?version=N) or the newest. It is an attachment, so
// the browser saves it instead of running it.
func (s *Server) exportArtifact(w http.ResponseWriter, r *http.Request) {
	version, ok := s.exportVersion(w, r)
	if !ok {
		return
	}
	d, err := s.export(r.Context(), currentViewer(r), baseURL(r), r.PathValue("id"), r.PathValue("format"), version)
	if refusal, ok := errors.AsType[*exportRefusal](err); ok {
		jsonError(w, refusal.status, refusal.Error())
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", d.ContentType)
	w.Header().Set("Content-Disposition", attachmentDisposition(d.Name))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(d.Body) // the client went away; nothing left to tell it
}

// exportVersion reads ?version=N, 0 when there is none. The artifact is checked
// first, so a malformed number never tells a stranger that the artifact exists.
func (s *Server) exportVersion(w http.ResponseWriter, r *http.Request) (int, bool) {
	if !r.URL.Query().Has("version") {
		return 0, true
	}
	n, err := strconv.Atoi(r.URL.Query().Get("version"))
	if err == nil && n >= 1 {
		return n, true
	}
	if _, ok := s.visibleArtifact(w, r); ok {
		jsonError(w, http.StatusBadRequest, "the version must be a number from 1; leave it out for the newest")
	}
	return 0, false
}

// exportRefusal is an export the server will not make for what the artifact
// is, as opposed to who asks: status is the HTTP answer, and the message is
// what both front ends say.
type exportRefusal struct {
	status  int
	refusal *service.Error
}

func (e *exportRefusal) Error() string { return e.refusal.Msg }
func (e *exportRefusal) Unwrap() error { return e.refusal }

func refuseExport(status int, format string, args ...any) error {
	return &exportRefusal{status: status, refusal: &service.Error{Code: service.Invalid, Msg: fmt.Sprintf(format, args...)}}
}

// export makes the download of one saved version of an artifact (0: the
// newest) as the viewer may have it. The HTTP route and the MCP export tool
// both call it, so the two give the same file and the same refusals.
func (s *Server) export(ctx context.Context, viewer service.Viewer, origin, id, format string, version int) (mcptools.Download, error) {
	a, err := s.svc.ArtifactAt(ctx, viewer, id, version)
	if err != nil {
		return mcptools.Download{}, err
	}
	k, err := kind.Lookup(a.Kind)
	if err != nil {
		return mcptools.Download{}, refuseExport(http.StatusNotImplemented, "this server cannot export artifacts of kind %s", a.Kind)
	}
	exporter, ok := k.(kind.Exporter)
	if !ok || !slices.Contains(exporter.Formats(), format) {
		var offered []string
		if ok {
			offered = exporter.Formats()
		}
		return mcptools.Download{}, refuseExport(http.StatusNotFound, "kind %s cannot export as %q; available formats: %s", a.Kind, format, strings.Join(offered, ", "))
	}
	if !kind.HasFormat(k, a.Format) {
		return mcptools.Download{}, refuseExport(http.StatusNotImplemented, "this server cannot export %s artifacts of format %s", a.Kind, a.Format)
	}
	bundler, bundles := exporter.(kind.BundleExporter)
	if len(a.Files) > 0 && !bundles {
		// The page refers to its files by relative path, so a download of the
		// page alone would open as a broken page; say so instead of handing it over.
		return mcptools.Download{}, refuseExport(http.StatusBadRequest, "this artifact has %d files besides its page, so downloading the page alone would give a broken page; "+
			"read the page and each file instead (GET /api/artifacts/%s/files/{path}), or copy the artifact", len(a.Files), a.ID)
	}
	var d kind.Download
	if bundles {
		var set files.Set
		if set, err = s.svc.FileTexts(ctx, a.ID, a.Version); err == nil {
			d, err = bundler.ExportBundle(a.Title, a.Content, a.Format, format, origin, set, s.exportFiles(ctx, viewer, origin, a.ID))
		}
	} else if fe, ok := exporter.(kind.FileExporter); ok {
		d, err = fe.ExportWithFiles(a.Content, a.Format, format, origin, s.exportFiles(ctx, viewer, origin, a.ID))
	} else {
		d, err = exporter.Export(a.Content, a.Format, format, origin)
	}
	if err != nil {
		return mcptools.Download{}, fmt.Errorf("export %s artifact %s version %d as %s: %w", a.Kind, a.ID, a.Version, format, err)
	}
	return mcptools.Download{
		Name: exportFileName(a.Title, a.ID, d.Extension), ContentType: d.ContentType, Body: d.Body, Version: a.Version,
		URL: origin + "/api/artifacts/" + url.PathEscape(a.ID) + "/export/" + url.PathEscape(format) + "?version=" + strconv.Itoa(a.Version),
	}, nil
}

// maxExportFileBytes bounds one picture read into an export.
const maxExportFileBytes = 20 << 20

// exportFiles finds the files of the artifact an export refers to, as the
// viewer who asked: an address under /files/{id}/ of this artifact, relative or
// whole. Anything else (another artifact, another server) is not found, and the
// export leaves it out.
func (s *Server) exportFiles(ctx context.Context, viewer service.Viewer, origin, id string) func(src string) (kind.File, bool) {
	prefix := "/files/" + id + "/"
	return func(src string) (kind.File, bool) {
		src = strings.TrimPrefix(src, origin)
		if kit, ok := platformKit(src); ok {
			return kit, true
		}
		if font, ok := strings.CutPrefix(src, "/static/fonts/"); ok {
			return platformFont(font)
		}
		name, ok := strings.CutPrefix(src, prefix)
		if !ok || name == "" || strings.Contains(name, "/") {
			return kind.File{}, false
		}
		rec, f, err := s.svc.OpenAsset(ctx, viewer, id, name)
		if err != nil {
			return kind.File{}, false
		}
		defer func() { _ = f.Close() }()
		data, err := io.ReadAll(io.LimitReader(f, maxExportFileBytes))
		if err != nil {
			return kind.File{}, false
		}
		return kind.File{Data: data, ContentType: rec.ContentType}, true
	}
}

// platformFont reads one of the fonts the server ships, which frames may use
// by address (/static/fonts/…).
func platformFont(name string) (kind.File, bool) {
	if name == "" || !fs.ValidPath("fonts/"+name) {
		return kind.File{}, false
	}
	data, err := fs.ReadFile(staticFS, "static/fonts/"+name)
	if err != nil {
		return kind.File{}, false
	}
	return kind.File{Data: data, ContentType: mime.TypeByExtension(path.Ext(name))}, true
}

// attachmentDisposition is the Content-Disposition of a download (RFC 6266): a
// quoted ASCII name every client reads, and the real name percent-encoded as
// UTF-8 in filename*, which clients that know it prefer. The name is always
// quoted, because a client that reads only filename="…" would otherwise find
// no name in a plain one (filename=Landing.zip).
func attachmentDisposition(name string) string {
	fallback := strings.Map(func(r rune) rune {
		if r < ' ' || r > '~' || strings.ContainsRune(`"\%`, r) {
			return '_'
		}
		return r
	}, name)
	return `attachment; filename="` + fallback + `"; filename*=UTF-8''` + encodeExtValue(name)
}

// encodeExtValue percent-encodes the UTF-8 of s except the characters RFC 5987
// allows bare, so a quote, a space, a parenthesis or an apostrophe never reaches
// the header as itself.
func encodeExtValue(s string) string {
	const attrChars = "!#$&+-.^_`|~"
	var b strings.Builder
	for _, c := range []byte(s) {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte(attrChars, c) >= 0 {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// exportFileName names a download after the artifact's title, so the file
// someone saves says what it is. Characters a file name cannot hold on some
// system (path separators, quotes, control characters) become a space, and a
// title with nothing left falls back to the id.
func exportFileName(title, id, extension string) string {
	cleaned := strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f || strings.ContainsRune(`/\:*?"<>|`, r) {
			return ' '
		}
		return r
	}, title)
	name := strings.Join(strings.Fields(cleaned), " ")
	if runes := []rune(name); len(runes) > 100 {
		name = strings.TrimSpace(string(runes[:100]))
	}
	name = strings.TrimLeft(name, ".") // not a hidden file
	if name == "" {
		name = id
	}
	return name + "." + extension
}

// routesExport registers downloads.
func (s *Server) routesExport(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/artifacts/{id}/export/{format}", s.api(s.exportArtifact))
}
