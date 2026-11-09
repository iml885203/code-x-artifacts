package docx

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif" // the pictures a doc may hold
	_ "image/jpeg"
	_ "image/png"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/docfmt"
)

// maxWidth is the width of the text on the page, in EMU (English Metric Units).
const maxWidth = 5731510

const emuPerPixel = 9525

// image writes a picture block as a drawing, or, when its bytes are not at
// hand (a picture on another server is never fetched), its description and a
// link to it.
func (w *writer) image(b docfmt.Block) string {
	pic, ok := w.picture(b.Src)
	if !ok {
		return w.imageLink(b)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(pic.Data))
	if err != nil || cfg.Width == 0 || cfg.Height == 0 {
		return w.imageLink(b)
	}
	cx, cy := int64(cfg.Width)*emuPerPixel, int64(cfg.Height)*emuPerPixel
	if cx > maxWidth {
		cy, cx = cy*maxWidth/cx, maxWidth
	}
	ext := map[string]string{"image/png": "png", "image/jpeg": "jpeg", "image/gif": "gif"}[pic.Type]
	n, seen := w.media[b.Src]
	if !seen {
		n = len(w.pictures) + 1
		w.media[b.Src] = n
		w.pictures = append(w.pictures, picture{path: fmt.Sprintf("word/media/image%d.%s", n, ext), data: pic.Data})
	}
	name := fmt.Sprintf("image%d.%s", n, ext)
	id := w.relate("http://schemas.openxmlformats.org/officeDocument/2006/relationships/image", "media/"+name, false)
	w.drawings++
	return fmt.Sprintf(`<w:r><w:drawing><wp:inline distT="0" distB="0" distL="0" distR="0"><wp:extent cx="%[1]d" cy="%[2]d"/><wp:docPr id="%[3]d" name="Picture %[3]d" descr="%[4]s"/>`+
		`<wp:cNvGraphicFramePr><a:graphicFrameLocks noChangeAspect="1"/></wp:cNvGraphicFramePr><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture">`+
		`<pic:pic><pic:nvPicPr><pic:cNvPr id="0" name="%[5]s" descr="%[4]s"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:embed="%[6]s"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill>`+
		`<pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%[1]d" cy="%[2]d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`,
		cx, cy, w.drawings, esc(b.Alt), name, id)
}

// picture finds the bytes behind an image address: a small data: address holds
// them itself, a file of the doc is asked for.
func (w *writer) picture(src string) (Image, bool) {
	if rest, ok := strings.CutPrefix(src, "data:"); ok {
		meta, payload, found := strings.Cut(rest, ",")
		mediaType, isBase64 := strings.CutSuffix(meta, ";base64")
		if !found || !isBase64 {
			return Image{}, false
		}
		data, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return Image{}, false
		}
		return Image{Data: data, Type: mediaType}, supported(mediaType)
	}
	if w.images == nil {
		return Image{}, false
	}
	pic, ok := w.images(src)
	return pic, ok && supported(pic.Type)
}

func supported(mediaType string) bool {
	return mediaType == "image/png" || mediaType == "image/jpeg" || mediaType == "image/gif"
}

// imageLink is what stands for a picture that is not in the file: [image: alt],
// and its address as a link when it has one that leaves the file.
func (w *writer) imageLink(b docfmt.Block) string {
	label := "[image"
	if b.Alt != "" {
		label += ": " + b.Alt
	}
	label += "]"
	if strings.HasPrefix(b.Src, "https://") || strings.HasPrefix(b.Src, "http://") {
		return w.runs([]docfmt.Inline{{Text: label, Marks: []string{docfmt.MarkLink}, Href: b.Src}}, false)
	}
	return plainRun(label)
}
