package web

import "github.com/iml885203/code-x-artifacts/internal/kind"

func platformKit(src string) (kind.File, bool) {
	for _, release := range kitVersions {
		if src == "/kit/"+release.dir+"/pictor.css" {
			return kind.File{Data: release.css, ContentType: "text/css"}, true
		}
	}
	return kind.File{}, false
}
