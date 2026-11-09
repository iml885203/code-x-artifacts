package mcptools

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/iml885203/code-x-artifacts/internal/service"
)

// Download is one export of an artifact: the file someone saves.
type Download struct {
	Name        string
	ContentType string
	Body        []byte
	// Version is the saved version the file was made from.
	Version int
	// URL is the same download through the HTTP export route.
	URL string
}

// Exporter makes the download the HTTP export route makes, for the same
// viewer: the web layer owns the exporters, and both front ends call the one
// it gives. version 0 is the newest. A refusal is a *service.Error.
type Exporter func(ctx context.Context, v service.Viewer, origin, id, format string, version int) (Download, error)

const exportDoc = `Download an artifact as a file, in one of the formats its kind offers (kinds lists them as exportFormats: html for a page,
md and docx for a doc, pptx for slides, zip for a design). PDF is not one: people print a page to PDF from their browser.
The result has the file's name (from the artifact's title), contentType, size, the version it was made from, and url.
A file of at most {limit} also comes inline: text for a text format (html, md), base64 for any other (docx, pptx, zip);
decode base64 before you save it. A larger file never comes inline, not even in part: the result then has note instead,
and you download url, which is the same download over HTTP and needs your CodeXArtifacts token:
curl -sS --fail-with-body -H "Authorization: Bearer <your CodeXArtifacts token>" -o "<name>" "<url>". The same command saves any file straight to disk.
version: optional; export that saved version instead of the newest (see versions). Older versions are for the author and editors,
as with read; anyone who can see the artifact may export the newest.`

type exportIn struct {
	ID      string `json:"id" jsonschema:"the artifact id: the last part of its url (.../a/<id>)"`
	Format  string `json:"format" jsonschema:"one of the exportFormats the kinds tool lists for the artifact's kind, such as md, docx, pptx, zip or html"`
	Version int    `json:"version,omitempty" jsonschema:"a saved version number to export (older versions are for authors and editors, see versions); omit for the newest"`
}

type exportOut struct {
	Name        string `json:"name"`
	ContentType string `json:"contentType"`
	Size        int    `json:"size"`
	Version     int    `json:"version"`
	URL         string `json:"url"`
	// Text is the file for a text format; Base64 the file for any other.
	// Both are empty for a file over the inline limit, and Note says why.
	Text   string `json:"text,omitempty"`
	Base64 string `json:"base64,omitempty"`
	Note   string `json:"note,omitempty"`
}

func (t *tools) export(ctx context.Context, _ *mcp.CallToolRequest, in exportIn) (*mcp.CallToolResult, exportOut, error) {
	d, err := t.e.export(ctx, t.viewer(ctx), t.caller(ctx).BaseURL, in.ID, in.Format, in.Version)
	if err != nil {
		return nil, exportOut{}, t.fail("export", err)
	}
	out := exportOut{Name: d.Name, ContentType: d.ContentType, Size: len(d.Body), Version: d.Version, URL: d.URL}
	switch {
	case len(d.Body) > t.e.maxUpload:
		out.Note = fmt.Sprintf("the file is %d bytes, more than the %s this tool hands over inline; download url with your CodeXArtifacts token: "+
			`curl -sS --fail-with-body -H "Authorization: Bearer <your CodeXArtifacts token>" -o %q %q`, len(d.Body), sizeText(t.e.maxUpload), d.Name, d.URL)
	case strings.HasPrefix(d.ContentType, "text/"):
		out.Text = string(d.Body)
	default:
		out.Base64 = base64.StdEncoding.EncodeToString(d.Body)
	}
	return nil, out, nil
}

// registerExport adds the tools of this feature to the server.
func (t *tools) registerExport(s *mcp.Server) {
	// The inline limit is upload_asset's: base64 travels through the agent's
	// context the same way in both directions.
	doc := strings.ReplaceAll(exportDoc, "{limit}", sizeText(t.e.maxUpload))
	mcp.AddTool(s, &mcp.Tool{Name: "export", Description: doc, Annotations: readOnly("Download an artifact as a file")}, t.export)
}
