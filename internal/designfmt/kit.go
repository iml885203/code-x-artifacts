package designfmt

import (
	"regexp"
	"strings"
)

var kitPath = regexp.MustCompile(`^/kit/v[1-5]/pictor\.css$`)

// IsKitStylesheet names only the platform's immutable, public kit releases.
func IsKitStylesheet(path string) bool { return kitPath.MatchString(path) }

func checkKitLink(attrs map[string]string, add func(string, ...any)) {
	if !strings.EqualFold(attrs["rel"], "stylesheet") || !IsKitStylesheet(attrs["href"]) {
		add("tag link is not allowed unless rel=stylesheet with href=/kit/v1/pictor.css through /kit/v5/pictor.css")
	}
	for key := range attrs {
		switch key {
		case "rel", "href", "media", "data-pid":
		default:
			add("attribute %s is not allowed on a kit link", key)
		}
	}
}

func permittedHref(tag, key, value string) bool {
	if key != "href" {
		return false
	}
	switch tag {
	case "link":
		return IsKitStylesheet(value)
	case "a":
		return isLink(value)
	}
	return false
}
