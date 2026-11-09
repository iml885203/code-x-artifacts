package dbquery

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFilterMatching(t *testing.T) {
	cases := []struct {
		name   string
		filter string
		id     string
		data   string
		want   bool
	}{
		{"empty filter matches everything", ``, "a", `{"x":1}`, true},
		{"null filter matches everything", `null`, "a", `{"x":1}`, true},
		{"empty object matches everything", `{}`, "a", `5`, true},
		{"equality on a string", `{"status":"open"}`, "a", `{"status":"open"}`, true},
		{"equality refuses another value", `{"status":"open"}`, "a", `{"status":"done"}`, false},
		{"equality does not coerce types", `{"n":1}`, "a", `{"n":"1"}`, false},
		{"equality on a number ignores how it is written", `{"n":1}`, "a", `{"n":1.0}`, true},
		{"equality on a bool", `{"done":false}`, "a", `{"done":false}`, true},
		{"a missing field equals nothing, not even null", `{"x":null}`, "a", `{}`, false},
		{"an explicit null equals null", `{"x":null}`, "a", `{"x":null}`, true},
		{"a dotted path reaches inside", `{"owner.name":"ada"}`, "a", `{"owner":{"name":"ada"}}`, true},
		{"a dotted path through a non-object does not match", `{"owner.name":"ada"}`, "a", `{"owner":"ada"}`, false},
		{"every key must hold", `{"a":1,"b":2}`, "x", `{"a":1,"b":3}`, false},
		{"every key holding matches", `{"a":1,"b":2}`, "x", `{"a":1,"b":2}`, true},
		{"id is matched against the document id", `{"id":"bob"}`, "bob", `{}`, true},
		{"id is not read from the data", `{"id":"bob"}`, "x", `{"id":"bob"}`, false},
		{"in matches a listed value", `{"s":{"in":["a","b"]}}`, "x", `{"s":"b"}`, true},
		{"in refuses an unlisted value", `{"s":{"in":["a","b"]}}`, "x", `{"s":"c"}`, false},
		{"in an empty list matches nothing", `{"s":{"in":[]}}`, "x", `{"s":"c"}`, false},
		{"gt on numbers", `{"n":{"gt":3}}`, "x", `{"n":4}`, true},
		{"gt is strict", `{"n":{"gt":3}}`, "x", `{"n":3}`, false},
		{"gte includes the bound", `{"n":{"gte":3}}`, "x", `{"n":3}`, true},
		{"lt on numbers", `{"n":{"lt":3}}`, "x", `{"n":2}`, true},
		{"lte includes the bound", `{"n":{"lte":3}}`, "x", `{"n":3}`, true},
		{"a range is the two bounds together", `{"n":{"gte":2,"lt":5}}`, "x", `{"n":5}`, false},
		{"a range holds inside its bounds", `{"n":{"gte":2,"lt":5}}`, "x", `{"n":2}`, true},
		{"numbers compare as numbers, not as text", `{"n":{"gt":9}}`, "x", `{"n":10}`, true},
		{"strings compare as strings", `{"d":{"gte":"2026-10-01"}}`, "x", `{"d":"2026-10-20"}`, true},
		{"a string is not above a number", `{"n":{"gt":1}}`, "x", `{"n":"5"}`, false},
		{"range on id", `{"id":{"gte":"b","lt":"d"}}`, "c", `{}`, true},
		{"prefix on a string", `{"name":{"prefix":"ad"}}`, "x", `{"name":"ada"}`, true},
		{"prefix refuses another start", `{"name":{"prefix":"ad"}}`, "x", `{"name":"bad"}`, false},
		{"prefix on a number does not match", `{"n":{"prefix":"1"}}`, "x", `{"n":12}`, false},
		{"prefix on id", `{"id":{"prefix":"2026-"}}`, "2026-10-20", `{}`, true},
		{"eq may hold an object", `{"o":{"eq":{"a":1,"b":[1,2]}}}`, "x", `{"o":{"b":[1,2],"a":1}}`, true},
		{"eq on an object refuses a different one", `{"o":{"eq":{"a":1}}}`, "x", `{"o":{"a":1,"b":2}}`, false},
		{"data that is not an object has no fields", `{"a":1}`, "x", `[1]`, false},
	}
	for _, c := range cases {
		f, err := Compile(json.RawMessage(c.filter))
		if err != nil {
			t.Errorf("%s: compile: %v", c.name, err)
			continue
		}
		if got := f.Match(c.id, json.RawMessage(c.data)); got != c.want {
			t.Errorf("%s: %s on %s = %v, want %v", c.name, c.filter, c.data, got, c.want)
		}
	}
}

func TestAFilterThatIsNotUnderstoodIsRefusedSayingWhatToSend(t *testing.T) {
	cases := []struct{ name, filter, says string }{
		{"not an object", `[1]`, "JSON object"},
		{"not JSON", `{`, "JSON object"},
		{"unknown operator", `{"n":{"between":[1,2]}}`, "unknown operator \"between\""},
		{"unknown operator lists the real ones", `{"n":{"between":[1,2]}}`, "gte, in, lt, lte, prefix"},
		{"empty operator object", `{"n":{}}`, "give an operator"},
		{"in needs a list", `{"n":{"in":"a"}}`, "takes a list"},
		{"prefix needs a string", `{"n":{"prefix":3}}`, "takes a string"},
		{"gt needs something comparable", `{"n":{"gt":true}}`, "number or a string"},
		{"empty field name", `{"":1}`, "not a field name"},
		{"empty path segment", `{"a..b":1}`, "not a field name"},
	}
	for _, c := range cases {
		_, err := Compile(json.RawMessage(c.filter))
		if err == nil || !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: err = %v, want it to say %q", c.name, err, c.says)
		}
	}
}

func TestMerge(t *testing.T) {
	cases := []struct{ name, data, patch, want string }{
		{"a field is added", `{"a":1}`, `{"b":2}`, `{"a":1,"b":2}`},
		{"a field is replaced", `{"a":1}`, `{"a":2}`, `{"a":2}`},
		{"null removes a field", `{"a":1,"b":2}`, `{"a":null}`, `{"b":2}`},
		{"null for a missing field changes nothing", `{"a":1}`, `{"z":null}`, `{"a":1}`},
		{"objects merge field by field", `{"o":{"a":1,"b":2}}`, `{"o":{"b":3,"c":4}}`, `{"o":{"a":1,"b":3,"c":4}}`},
		{"null inside an object removes that field", `{"o":{"a":1,"b":2}}`, `{"o":{"a":null}}`, `{"o":{"b":2}}`},
		{"an array is replaced whole", `{"l":[1,2,3]}`, `{"l":[9]}`, `{"l":[9]}`},
		{"an object replaces a scalar", `{"o":1}`, `{"o":{"a":1}}`, `{"o":{"a":1}}`},
		{"a scalar replaces an object", `{"o":{"a":1}}`, `{"o":5}`, `{"o":5}`},
		{"an empty patch changes nothing", `{"a":1}`, `{}`, `{"a":1}`},
	}
	for _, c := range cases {
		got, err := Merge(json.RawMessage(c.data), json.RawMessage(c.patch))
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if string(got) != c.want {
			t.Errorf("%s: %s + %s = %s, want %s", c.name, c.data, c.patch, got, c.want)
		}
	}
}

func TestMergeRefusesWhatHasNoFields(t *testing.T) {
	for _, c := range []struct{ name, data, patch, says string }{
		{"the document is an array", `[1]`, `{"a":1}`, "not a JSON object"},
		{"the document is a scalar", `"x"`, `{"a":1}`, "not a JSON object"},
		{"the document is null", `null`, `{"a":1}`, "not a JSON object"},
		{"the fields are an array", `{"a":1}`, `[1]`, "must be a JSON object"},
		{"the fields are null", `{"a":1}`, `null`, "must be a JSON object"},
	} {
		if _, err := Merge(json.RawMessage(c.data), json.RawMessage(c.patch)); err == nil || !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: err = %v, want it to say %q", c.name, err, c.says)
		}
	}
}
