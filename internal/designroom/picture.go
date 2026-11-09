package designroom

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
)

// A picture on an element is shown by `background-image` (an image fill) or, on
// an <img>, by `object-fit`. Both are owned by the editor so that two people
// changing a fill and its fit keep both. The value of background-image is the
// one place a style entry may name an address, so it has a grammar of its own:
// none, or a list of url() whose address is one a frame may point at
// (designfmt's rule: a fragment, a small picture written into the markup, or a
// file uploaded to the artifact, /files/<id>/<name>).
const pictureProps = "background-image background-size background-position background-repeat object-fit"

var pictureSet = setOf2(pictureProps)

// addressAlphabet is what an address in a url() may be made of: nothing that
// could end the declaration or the rule, or write a quote or an escape.
var addressAlphabet = regexp.MustCompile(`^[A-Za-z0-9._~%/:;,+=#-]*$`)

// checkBackgroundImage holds the value of a background-image entry to the grammar.
func checkBackgroundImage(v string) error {
	if len(v) > maxValueLen {
		return fmt.Errorf("a value is at most %d bytes", maxValueLen)
	}
	if strings.EqualFold(v, "none") || in(nil, v) {
		return nil
	}
	rest := v
	for {
		address, after, err := urlArgument(strings.TrimSpace(rest), v)
		if err != nil {
			return err
		}
		if err := checkPictureAddress(address); err != nil {
			return err
		}
		after = strings.TrimSpace(after)
		if after == "" {
			return nil
		}
		if after[0] != ',' {
			return fmt.Errorf("%q: only url() pictures, separated by commas", v)
		}
		rest = after[1:]
	}
}

// urlArgument is the address inside the url( that starts text, and what follows
// the closing parenthesis; whole is the value, for the message.
func urlArgument(text, whole string) (address, after string, err error) {
	if !strings.HasPrefix(strings.ToLower(text), "url(") {
		return "", "", fmt.Errorf("%q is not a picture of this artifact: write none or url(/files/<id>/<name>)", whole)
	}
	text = strings.TrimSpace(text[len("url("):])
	if text != "" && (text[0] == '"' || text[0] == '\'') {
		end := strings.IndexByte(text[1:], text[0])
		if end < 0 {
			return "", "", fmt.Errorf("a quote is not closed")
		}
		after = strings.TrimSpace(text[end+2:])
		if !strings.HasPrefix(after, ")") {
			return "", "", fmt.Errorf("a url( is not closed")
		}
		return text[1 : 1+end], after[1:], nil
	}
	end := strings.IndexByte(text, ')')
	if end < 0 {
		return "", "", fmt.Errorf("a url( is not closed")
	}
	return strings.TrimSpace(text[:end]), text[end+1:], nil
}

// checkPictureAddress holds an address to the alphabet of one and to what a frame may point at.
func checkPictureAddress(address string) error {
	if !addressAlphabet.MatchString(address) {
		return fmt.Errorf("the address %.60q has a character an address of a frame does not", address)
	}
	if problems := designfmt.CheckAttribute("img", "src", address); len(problems) > 0 {
		return fmt.Errorf("%s", problems[0])
	}
	return nil
}
