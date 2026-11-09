package designroom

import (
	"fmt"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/designfmt"
)

const tokenPrefix = "t|"

func tokenEntries(m designfmt.Manifest, out map[string]string) {
	for name, value := range m.Tokens {
		out[tokenPrefix+name] = value
	}
}

func tokensOf(e map[string]any) map[string]string {
	out := map[string]string{}
	for key, value := range e {
		if name, ok := strings.CutPrefix(key, tokenPrefix); ok {
			out[name] = str(value)
		}
	}
	return out
}

func checkTokenEntries(e map[string]any) error {
	for key, value := range e {
		name, ok := strings.CutPrefix(key, tokenPrefix)
		if !ok {
			continue
		}
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("tokens[%q]: value must be a CSS string", name)
		}
		if err := designfmt.ValidateToken(name, text); err != nil {
			return err
		}
	}
	return nil
}
