package kind

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
)

// DesignKitFrame inlines only the trusted kit so frames keep their inline-only
// style policy and exported designs do not depend on the server.
func DesignKitFrame(markup string, open func(string) (File, bool)) (string, error) {
	z := html.NewTokenizer(strings.NewReader(markup))
	var out strings.Builder
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		raw := string(z.Raw())
		if tt == html.StartTagToken || tt == html.SelfClosingTagToken {
			token := z.Token()
			if token.Data == "link" {
				var href, media string
				for _, attr := range token.Attr {
					if attr.Key == "href" {
						href = attr.Val
					}
					if attr.Key == "media" {
						media = attr.Val
					}
				}
				if designfmt.IsKitStylesheet(href) {
					var css File
					var ok bool
					if open != nil {
						css, ok = open(href)
					}
					if !ok {
						return "", fmt.Errorf("read design kit stylesheet %s", href)
					}
					raw = `<style media="` + html.EscapeString(media) + `">` + string(css.Data) + `</style>`
				}
			}
		}
		out.WriteString(raw)
	}
	return out.String(), nil
}
