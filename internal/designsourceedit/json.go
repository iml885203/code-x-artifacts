package designsourceedit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type member struct {
	key        string
	start, end int
	raw        json.RawMessage
}

func members(source string) ([]member, error) {
	d := json.NewDecoder(strings.NewReader(source))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, fmt.Errorf("expected JSON object")
	}
	var out []member
	seen := map[string]bool{}
	for d.More() {
		k, e := d.Token()
		if e != nil {
			return nil, e
		}
		key, ok := k.(string)
		if !ok || seen[key] {
			return nil, fmt.Errorf("invalid or duplicate object key")
		}
		seen[key] = true
		var raw json.RawMessage
		if e := d.Decode(&raw); e != nil {
			return nil, e
		}
		end := int(d.InputOffset())
		out = append(out, member{key, end - len(raw), end, raw})
	}
	if _, err := d.Token(); err != nil {
		return nil, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("extra JSON input")
	}
	return out, nil
}

func canonical(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var value any
	if err := d.Decode(&value); err != nil {
		return "", err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return "", fmt.Errorf("extra JSON input")
	}
	b, err := json.Marshal(value)
	return string(b), err
}

// ValueDigest distinguishes an absent authored value from explicit JSON null.
// Invalid JSON returns an empty digest, which no operation can satisfy.
func ValueDigest(raw json.RawMessage) string {
	c, err := canonical(raw)
	if err != nil {
		return ""
	}
	if len(raw) == 0 {
		return Digest("pictor-value/1:absent")
	}
	return Digest("pictor-value/1:present:" + c)
}

// UnitDigest includes logic and schema/default declarations, never authored
// values. An absent logic/schema pair has an explicit digest for creation CAS.
func UnitDigest(logic string, schema json.RawMessage) string {
	c, err := canonical(schema)
	if err != nil {
		return ""
	}
	b, _ := json.Marshal([]string{"pictor-logic-declaration/1", logic, c})
	return Digest(string(b))
}

func field(source, key string) (member, bool, error) {
	fields, err := members(source)
	if err != nil {
		return member{}, false, err
	}
	for _, f := range fields {
		if f.key == key {
			return f, true, nil
		}
	}
	return member{}, false, nil
}

func replaceField(source, key string, value json.RawMessage) (string, error) {
	fields, err := members(source)
	if err != nil {
		return "", err
	}
	for i, f := range fields {
		if f.key != key {
			continue
		}
		if len(value) > 0 {
			return source[:f.start] + string(value) + source[f.end:], nil
		}
		// Include only this key's own separator; keep other field bytes intact.
		start := 0
		if i > 0 {
			start = fields[i-1].end
		} else {
			start = strings.IndexByte(source, '{') + 1
		}
		if i+1 < len(fields) {
			end := f.end
			for end < len(source) && source[end] != ',' {
				end++
			}
			if i == 0 {
				return source[:start] + source[end+1:], nil
			}
		}
		return source[:start] + source[f.end:], nil
	}
	if len(value) == 0 {
		return source, nil
	}
	at := strings.LastIndexByte(source, '}')
	keyJSON, _ := json.Marshal(key)
	prefix := ""
	if len(fields) > 0 {
		prefix = ","
		at = fields[len(fields)-1].end
	}
	return source[:at] + prefix + string(keyJSON) + ":" + string(value) + source[at:], nil
}
