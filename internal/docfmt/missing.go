package docfmt

import (
	"html"
	"net/url"
	"regexp"
	"strings"
)

// MissingPictures draws the placeholder where a doc shows one of the
// artifact's own uploaded files that is no longer there. Uploads belong to the
// artifact, not to its versions, so an older version (or a doc whose file was
// deleted) can name a file that is gone; the page then says so instead of
// showing a broken picture. It only reads markup RenderBody wrote, and an
// address of another site is left to the browser.
func MissingPictures(page, artifactID string, uploaded func(name string) bool) string {
	pattern := regexp.MustCompile(`<p class="image"( data-block="[^"]*")><img src="(?:https?://[^"/]+)?/files/` + regexp.QuoteMeta(artifactID) + `/([^"/]+)" alt="([^"]*)"(?: width="\d+")? loading="lazy"></p>`)
	return pattern.ReplaceAllStringFunc(page, func(whole string) string {
		m := pattern.FindStringSubmatch(whole)
		name, err := url.PathUnescape(html.UnescapeString(m[2]))
		if err != nil || uploaded(name) {
			return whole
		}
		note := "Picture deleted"
		if language(page) == "zh-Hant" {
			note = "圖片已刪除"
		}
		if alt := strings.TrimSpace(html.UnescapeString(m[3])); alt != "" {
			note += ": " + alt
		}
		return `<p class="missing"` + m[1] + ">" + html.EscapeString(note) + "</p>"
	})
}
