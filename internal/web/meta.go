package web

import (
	"mime"

	"github.com/iml885203/code-x-artifacts/internal/i18n"
)

// Go does not know the manifest's type, and a browser ignores a manifest sent as anything else.
var _ = mime.AddExtensionType(".webmanifest", "application/manifest+json")

// pageTitle is the tab's title and the share title of a shell page, in the
// language served: "name · CodeXArtifacts", or just "CodeXArtifacts" for the overview. It is
// what the app sets again on its own (web/src/title.ts), so the tab does not
// change when the app starts. An artifact's title is in it only when the page
// carries the artifact at all, and the server hands that to nobody who may not
// see it: they get the error page, whose title says only what is true of both.
func pageTitle(lang *i18n.Lang, d pageData) string {
	name := ""
	switch d.page {
	case "login":
		name = lang.T("login.title")
	case "connect":
		name = lang.T("connect.title")
	case "kit":
		name = lang.T("kit.title")
	case "error":
		name = lang.T("error.notFound.title")
	case "viewer":
		if d.artifact != nil {
			name = d.artifact.Title
		}
	}
	if name == "" {
		return "CodeXArtifacts"
	}
	return name + " · CodeXArtifacts"
}
