package designsource

import (
	"encoding/json"
	"fmt"
	"strings"
)

// CheckData bounds JSON shared by props, render values and repeat items before
// projection. No reflection, custom marshalers, getters or callable objects run.
func CheckData(value any) error {
	leaves := 0
	bytes := 0
	if e := checkData(value, 0, &leaves, &bytes); e != nil {
		return e
	}
	if bytes > 256<<10 {
		return fmt.Errorf("data exceeds 256 KiB")
	}
	encoded, e := json.Marshal(value)
	if e != nil {
		return e
	}
	if len(encoded) > 256<<10 {
		return fmt.Errorf("serialized data exceeds 256 KiB")
	}
	return nil
}

func checkData(value any, depth int, leaves, bytes *int) error {
	if depth > 16 {
		return fmt.Errorf("data exceeds depth 16")
	}
	switch v := value.(type) {
	case map[string]any:
		return checkObject(v, depth, leaves, bytes)
	case []any:
		*bytes += 2 + len(v)
		if *bytes > 256<<10 {
			return fmt.Errorf("data exceeds 256 KiB")
		}
		for _, item := range v {
			if e := checkData(item, depth+1, leaves, bytes); e != nil {
				return e
			}
		}
	default:
		text, e := scalar(value)
		if e != nil {
			return e
		}
		*bytes += len(text)
		*leaves++
		if *leaves > 10000 {
			return fmt.Errorf("data exceeds 10000 primitive leaves")
		}
	}
	if *bytes > 256<<10 {
		return fmt.Errorf("data exceeds 256 KiB")
	}
	return nil
}

// checkUniqueJSON prevents ambiguous duplicate-key source/CAS interpretations.
func checkUniqueJSON(source string) error {
	decoder := json.NewDecoder(strings.NewReader(source))
	decoder.UseNumber()
	return jsonValue(decoder, 0)
}

func jsonValue(d *json.Decoder, depth int) error {
	if depth > 128 {
		return fmt.Errorf("JSON nesting exceeds 128")
	}
	token, e := d.Token()
	if e != nil {
		return e
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if delimiter == '{' {
		return jsonObject(d, depth)
	}
	if delimiter == '[' {
		for d.More() {
			if e := jsonValue(d, depth+1); e != nil {
				return e
			}
		}
		_, e = d.Token()
		return e
	}
	return fmt.Errorf("unexpected JSON delimiter")
}

func jsonObject(d *json.Decoder, depth int) error {
	seen := map[string]bool{}
	for d.More() {
		token, e := d.Token()
		if e != nil {
			return e
		}
		key, ok := token.(string)
		if !ok {
			return fmt.Errorf("object key must be a string")
		}
		if seen[key] {
			return fmt.Errorf("duplicate JSON key %q at byte %d", key, d.InputOffset())
		}
		seen[key] = true
		if e := jsonValue(d, depth+1); e != nil {
			return e
		}
	}
	_, e := d.Token()
	return e
}

func checkObject(v map[string]any, depth int, leaves, bytes *int) error {
	*bytes += 2 + len(v)*4
	if *bytes > 256<<10 {
		return fmt.Errorf("data exceeds 256 KiB")
	}
	for _, key := range sortedKeys(v) {
		if key == "__proto__" || key == "prototype" || key == "constructor" {
			return fmt.Errorf("data contains a reserved prototype key")
		}
		*bytes += len(key)
		if e := checkData(v[key], depth+1, leaves, bytes); e != nil {
			return e
		}
	}
	return nil
}
