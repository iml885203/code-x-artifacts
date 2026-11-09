package kind

import "github.com/iml885203/code-x-artifacts/internal/designfmt"

// DesignTokenFrame adds the manifest's variables to a rendered copy of a frame.
// The stored source retains var() bindings and never acquires generated styles.
func DesignTokenFrame(content, markup string) string {
	m, problems := designfmt.Parse(content)
	if len(problems) > 0 || len(m.Tokens) == 0 {
		return markup
	}
	tag := `<style data-pictor-tokens>` + designfmt.TokenCSS(m.Tokens) + `</style>`
	// Last in the head lets the shared theme override authored root defaults.
	if loc := frameHeadClose.FindStringIndex(markup); loc != nil {
		return markup[:loc[0]] + tag + markup[loc[0]:]
	}
	if loc := frameDoctype.FindStringIndex(markup); loc != nil {
		return markup[:loc[1]] + tag + markup[loc[1]:]
	}
	return tag + markup
}
