package mcptools

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/iml885203/code-x-artifacts/internal/assets"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// DefaultMaxUploadBytes is the largest file the upload_asset tool takes,
// decoded. Base64 travels through the agent's context, so it is lower than the
// HTTP API's limit. Config.MaxUploadBytes overrides it.
const DefaultMaxUploadBytes = 5 << 20

// sizeText writes a byte limit the way the tool's messages quote it: whole MB
// when it is one, otherwise KB.
func sizeText(n int) string {
	if n%(1<<20) == 0 {
		return fmt.Sprintf("%d MB", n>>20)
	}
	return fmt.Sprintf("%d KB", (n+1023)>>10)
}

// assetOut mirrors the HTTP API's asset: the same fields for the same viewer.
type assetOut struct {
	store.Asset
	URL string `json:"url"`
}

func (t *tools) asset(ctx context.Context, id string, a store.Asset) assetOut {
	return assetOut{Asset: a, URL: t.caller(ctx).BaseURL + assets.Path(id, a.Name)}
}

const assetIntro = `An artifact can hold files (images, fonts, PDFs, CSV) that its page references by url, so a report or a deck shows
real pictures without putting base64 into the page. `

const uploadAssetDoc = assetIntro + `upload_asset stores one file on an artifact you may edit (you authored it, or the author made you an editor) and returns the url to put in the page.
Use it when the page needs a chart, a screenshot, a logo, a font or a data file: make the file, upload it, then write
the url into the content and publish (or update) the artifact. The artifact must exist first.
Accepted: ` + assets.Allowed + `. The type is decided from the file's bytes, not from the name you give, and the name's
extension must agree (a PNG is chart.png). Refused: anything else, an empty file, a file over {limit} through this tool
(the HTTP API takes 15 MB: PUT /api/artifacts/{id}/assets/{name}), and more than 50 files or 100 MB on one artifact.
name: the file name, such as chart.png. It is cleaned to letters, digits, dot, dash and underscore; the result's
name is the real one, and the url is built from it. Uploading a name that exists replaces that file, so the same url
shows the new picture after a day's browser cache; use a new name when the old picture must not change.
content_base64: the file's bytes, base64-encoded (standard alphabet, no line breaks needed).
Put the returned url exactly as it is into the page (html: <img src="URL">, css url(URL), fetch(URL); doc:
![alt](URL)). It works for everyone who can open the artifact, and the page's own scripts can fetch it. Who can see a
file is who can see the artifact: change the audience with set_visibility and the files follow. Deleting the
artifact deletes its files.`

type uploadAssetIn struct {
	ID            string `json:"id" jsonschema:"the artifact id (it must be yours)"`
	Name          string `json:"name" jsonschema:"file name with extension, such as chart.png; cleaned to letters, digits, dot, dash, underscore"`
	ContentBase64 string `json:"content_base64" jsonschema:"the file's bytes, base64-encoded, at most 5 MB decoded"`
}

type uploadAssetOut struct {
	assetOut
	// Replaced is true when a file of that name was already there.
	Replaced bool `json:"replaced"`
	// Next is how to use the url.
	Next string `json:"next"`
}

func (t *tools) uploadAsset(ctx context.Context, _ *mcp.CallToolRequest, in uploadAssetIn) (*mcp.CallToolResult, uploadAssetOut, error) {
	data, err := t.e.decodeUpload(in.ContentBase64)
	if err != nil {
		return nil, uploadAssetOut{}, err
	}
	up, err := t.e.svc.UploadAsset(ctx, t.viewer(ctx), in.ID, in.Name, data)
	if err != nil {
		return nil, uploadAssetOut{}, t.fail("upload_asset", err)
	}
	out := uploadAssetOut{assetOut: t.asset(ctx, in.ID, up.Asset), Replaced: up.Replaced}
	out.Next = fmt.Sprintf("Stored as %s (%s, %d bytes). Reference exactly this url in the artifact's content, then publish it with the id and baseVersion you have.", up.Name, up.ContentType, up.Size)
	if up.Replaced {
		out.Next += " A file with this name was replaced; browsers may show the old one for up to a day."
	}
	return nil, out, nil
}

// decodeUpload checks the size before decoding, so an oversized payload costs
// no memory, and accepts the two things agents commonly send: line-wrapped
// base64 and a data: URL prefix.
func (e *Endpoint) decodeUpload(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "data:") {
		if _, rest, ok := strings.Cut(s, ";base64,"); ok {
			s = rest
		}
	}
	s = strings.NewReplacer("\n", "", "\r", "", " ", "").Replace(s)
	if s == "" {
		return nil, errors.New("content_base64 is required: the file's bytes, base64-encoded")
	}
	if len(s) > base64.StdEncoding.EncodedLen(e.maxUpload) {
		return nil, fmt.Errorf("the file is larger than the %s this tool takes; shrink it (for an image, a smaller resolution or a PNG instead of a BMP) or upload it over the HTTP API, which takes %d MB", sizeText(e.maxUpload), assets.MaxBytes>>20)
	}
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		data, err = base64.RawStdEncoding.DecodeString(s)
	}
	if err != nil {
		return nil, errors.New("content_base64 is not valid base64: send the file's bytes with the standard base64 alphabet, and nothing else (no JSON, no file path)")
	}
	if len(data) > e.maxUpload {
		return nil, fmt.Errorf("the file is %d bytes; this tool takes at most %s", len(data), sizeText(e.maxUpload))
	}
	return data, nil
}

const listAssetsDoc = assetIntro + `list_assets lists the files of an artifact you can open, each with the url the page uses, its content type and
size. Use it to find a file you uploaded earlier before uploading it again, or to see what a page can reference.`

type listAssetsOut struct {
	Assets []assetOut `json:"assets"`
}

func (t *tools) listAssets(ctx context.Context, _ *mcp.CallToolRequest, in idIn) (*mcp.CallToolResult, listAssetsOut, error) {
	list, err := t.e.svc.Assets(ctx, t.viewer(ctx), in.ID)
	if err != nil {
		return nil, listAssetsOut{}, t.fail("list_assets", err)
	}
	out := listAssetsOut{Assets: make([]assetOut, 0, len(list))}
	for _, a := range list {
		out.Assets = append(out.Assets, t.asset(ctx, in.ID, a))
	}
	return nil, out, nil
}

const deleteAssetDoc = assetIntro + `delete_asset removes one file from an artifact you may edit (you authored it, or the author made you an editor). It cannot be undone, and any page that still
references its url shows a broken picture, so remove the reference from the page first or only when the person asked.
name: the file's name as list_assets or upload_asset returned it.`

type deleteAssetIn struct {
	ID   string `json:"id" jsonschema:"the artifact id (it must be yours)"`
	Name string `json:"name" jsonschema:"the file name, as list_assets returned it"`
}

func (t *tools) deleteAsset(ctx context.Context, _ *mcp.CallToolRequest, in deleteAssetIn) (*mcp.CallToolResult, deletedOut, error) {
	if err := t.e.svc.DeleteAsset(ctx, t.viewer(ctx), in.ID, in.Name); err != nil {
		return nil, deletedOut{}, t.fail("delete_asset", err)
	}
	return nil, deletedOut{Deleted: true}, nil
}

// registerAssets adds the tools of this feature to the server.
func (t *tools) registerAssets(s *mcp.Server) {
	uploadDoc := strings.ReplaceAll(uploadAssetDoc, "{limit}", sizeText(t.e.maxUpload))
	mcp.AddTool(s, &mcp.Tool{Name: "upload_asset", Description: uploadDoc, Annotations: &mcp.ToolAnnotations{Title: "Upload an image or file to an artifact", DestructiveHint: ptr(true), IdempotentHint: true, OpenWorldHint: ptr(false)}}, t.uploadAsset)
	mcp.AddTool(s, &mcp.Tool{Name: "list_assets", Description: listAssetsDoc, Annotations: readOnly("List an artifact's files")}, t.listAssets)
	mcp.AddTool(s, &mcp.Tool{Name: "delete_asset", Description: deleteAssetDoc, Annotations: &mcp.ToolAnnotations{Title: "Delete a file from an artifact", DestructiveHint: ptr(true), IdempotentHint: true, OpenWorldHint: ptr(false)}}, t.deleteAsset)
}
