package docfmt

import (
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Dropdown is one selected label and the bounded set it can be chosen from.
type Dropdown struct {
	Options []string `json:"options"`
	Value   string   `json:"value"`
}

// MaxDropdownOptions and MaxDropdownLabelRunes bound the shared dropdown configuration.
const (
	MaxDropdownOptions    = 50
	MaxDropdownLabelRunes = 100
)

// JSON is the atomic room attribute, in the same order as the shared schema.
func (d Dropdown) JSON() string {
	out, _ := marshal(d) // Strings and slices of strings cannot fail JSON encoding.
	return out
}

// ParseDropdown validates a room attribute before it becomes an inline run.
func ParseDropdown(raw string) (*Dropdown, error) {
	value, err := decode([]byte(raw))
	if err != nil {
		return nil, err
	}
	if err := checkDropdownConfig(value, "dropdown"); err != nil {
		return nil, err
	}
	var config Dropdown
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func checkDropdown(obj map[string]any, path string) error {
	for _, key := range []string{"text", "marks", "href", "user", "name", "date"} {
		if _, has := obj[key]; has {
			return at(path+"."+key, "a dropdown has only its dropdown configuration")
		}
	}
	return checkDropdownConfig(obj["dropdown"], path+".dropdown")
}

func checkDropdownConfig(raw any, path string) error {
	config, err := object(raw, path, "options", "value")
	if err != nil {
		return err
	}
	options, err := list(config["options"], path+".options")
	if err != nil {
		return err
	}
	if len(options) < 1 || len(options) > MaxDropdownOptions {
		return at(path+".options", "has %d options; between 1 and %d are required", len(options), MaxDropdownOptions)
	}
	seen := map[string]bool{}
	for i, raw := range options {
		where := idx(path+".options", i)
		option, err := dropdownOption(raw, where)
		if err != nil {
			return err
		}
		if seen[option] {
			return at(where, "must be distinct; %q is repeated", option)
		}
		seen[option] = true
	}
	value, ok := config["value"].(string)
	if !ok || !seen[value] {
		return at(path+".value", "must equal one of the options")
	}
	return nil
}

func dropdownOption(raw any, where string) (string, error) {
	option, ok := raw.(string)
	if !ok {
		return "", at(where, "must be text")
	}
	if option == "" || strings.TrimFunc(option, func(r rune) bool { return unicode.IsSpace(r) || r == '\uFEFF' }) != option {
		return "", at(where, "must be nonempty and trimmed")
	}
	if strings.ContainsAny(option, "\r\n\u2028\u2029") || strings.ContainsFunc(option, unicode.IsControl) {
		return "", at(where, "must be one line without control characters")
	}
	if count := utf8.RuneCountInString(option); count > MaxDropdownLabelRunes {
		return "", at(where, "is %d characters; at most %d", count, MaxDropdownLabelRunes)
	}
	return option, nil
}
