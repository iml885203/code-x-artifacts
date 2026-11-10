package docfmt

import (
	"reflect"
	"strings"
	"testing"
)

const anna = `{"user":"anna.lee","name":"Anna Lee"}`

func TestAMentionIsValidWhereverTextIs(t *testing.T) {
	t.Parallel()
	for name, blocks := range map[string]string{
		"paragraph":       `{"id":"b1","type":"paragraph","content":[{"text":"Ask "},` + anna + `,{"text":" to confirm"}]}`,
		"alone":           `{"id":"b1","type":"paragraph","content":[` + anna + `]}`,
		"heading":         `{"id":"b1","type":"heading","level":2,"content":[` + anna + `]}`,
		"list item":       `{"id":"b1","type":"bullet_list","items":[{"content":[` + anna + `],"items":[{"content":[{"text":"x "},` + anna + `]}]}]}`,
		"task item":       `{"id":"b1","type":"task_list","items":[{"checked":false,"content":[` + anna + `,{"text":" to review"}]}]}`,
		"table cell":      `{"id":"b1","type":"table","rows":[{"cells":[{"content":[` + anna + `]}]}]}`,
		"quote":           `{"id":"b1","type":"quote","blocks":[{"id":"b2","type":"paragraph","content":[` + anna + `]}]}`,
		"next to marks":   `{"id":"b1","type":"paragraph","content":[{"text":"bold","marks":["bold"]},` + anna + `,{"text":"link","marks":["link"],"href":"https://example.com"}]}`,
		"two in a row":    `{"id":"b1","type":"paragraph","content":[` + anna + `,` + anna + `]}`,
		"name of a CJK":   `{"id":"b1","type":"paragraph","content":[{"user":"wang.xm","name":"王小明 😀"}]}`,
		"name at limit":   `{"id":"b1","type":"paragraph","content":[{"user":"` + strings.Repeat("a", MaxMentionUserRunes) + `","name":"` + strings.Repeat("n", MaxMentionNameRunes) + `"}]}`,
		"account symbols": `{"id":"b1","type":"paragraph","content":[{"user":"A.b_c-9","name":"A"}]}`,
	} {
		if err := Validate(one(blocks)); err != nil {
			t.Errorf("%s: refused: %v", name, err)
		}
	}
}

func TestAMentionThatBreaksTheRulesIsRefusedWithThePathOfTheProblem(t *testing.T) {
	t.Parallel()
	const head = `{"id":"b1","type":"paragraph","content":[{"text":"a"},`
	for name, tc := range map[string]struct{ run, want string }{
		"no account":           {`{"name":"Anna"}`, "tabs[0].blocks[0].content[1].user: required"},
		"no name":              {`{"user":"anna"}`, "tabs[0].blocks[0].content[1].name: required"},
		"empty account":        {`{"user":"","name":"Anna"}`, "tabs[0].blocks[0].content[1].user: must not be empty"},
		"null account":         {`{"user":null,"name":"Anna"}`, "tabs[0].blocks[0].content[1].user: required"},
		"blank name":           {`{"user":"anna","name":"  "}`, "tabs[0].blocks[0].content[1].name: must not be empty"},
		"name control":         {`{"user":"anna","name":"An\tna"}`, "tabs[0].blocks[0].content[1].name: must not contain control characters"},
		"empty name":           {`{"user":"anna","name":""}`, "tabs[0].blocks[0].content[1].name: must not be empty"},
		"account with a space": {`{"user":"anna lee","name":"Anna"}`, "tabs[0].blocks[0].content[1].user: must be letters, digits"},
		"account with symbols": {`{"user":"anna](x)","name":"Anna"}`, "tabs[0].blocks[0].content[1].user: must be letters, digits"},
		"account too long":     {`{"user":"` + strings.Repeat("a", MaxMentionUserRunes+1) + `","name":"Anna"}`, "tabs[0].blocks[0].content[1].user: is 65 characters; at most 64"},
		"name two lines":       {`{"user":"anna","name":"An\nna"}`, "tabs[0].blocks[0].content[1].name: must be one line"},
		"name too long":        {`{"user":"anna","name":"` + strings.Repeat("n", MaxMentionNameRunes+1) + `"}`, "tabs[0].blocks[0].content[1].name: is 101 characters; at most 100"},
		"name with NUL":        {`{"user":"anna","name":"a\u0000b"}`, "tabs[0].blocks[0].content[1].name: must not contain a NUL"},
		"name not text":        {`{"user":"anna","name":5}`, "tabs[0].blocks[0].content[1].name: must be text"},
		"account not text":     {`{"user":5,"name":"Anna"}`, "tabs[0].blocks[0].content[1].user: must be text"},
		"text beside account":  {`{"user":"anna","name":"Anna","text":"x"}`, "tabs[0].blocks[0].content[1].text: a mention has no text"},
		"marks on a mention":   {`{"user":"anna","name":"Anna","marks":["bold"]}`, "tabs[0].blocks[0].content[1].marks: a mention has no marks"},
		"href on a mention":    {`{"user":"anna","name":"Anna","href":"https://example.com"}`, "tabs[0].blocks[0].content[1].href: a mention has no address"},
		"name without user":    {`{"text":"x","name":"Anna"}`, "tabs[0].blocks[0].content[1].name: only a mention has one"},
		"unknown field":        {`{"user":"anna","name":"Anna","email":"a@b.co"}`, `tabs[0].blocks[0].content[1]: unknown field "email"`},
	} {
		err := Validate(one(head + tc.run + `]}`))
		switch {
		case err == nil:
			t.Errorf("%s: accepted, want a refusal mentioning %q", name, tc.want)
		case !strings.Contains(err.Error(), tc.want):
			t.Errorf("%s: error %q does not mention %q", name, err, tc.want)
		}
	}
	if err := Validate(one(`{"id":"b1","type":"table","rows":[{"cells":[{"content":[{"user":"a b","name":"x"}]}]}]}`)); err == nil || !strings.Contains(err.Error(), "tabs[0].blocks[0].rows[0].cells[0].content[0].user") {
		t.Errorf("a bad mention in a cell is refused with its path: %v", err)
	}
}

func TestRenderShowsAMentionAsItsNameWithAnAtAndNoMarkupFromTheName(t *testing.T) {
	t.Parallel()
	const evil = `<script>alert(1)</script><img src=x onerror=alert(2)>" onmouseover="alert(3)`
	d := mustParse(t, one(`{"id":"p","type":"paragraph","content":[{"text":"Ask "},`+anna+`,{"text":"!"}]},
{"id":"h","type":"heading","level":1,"content":[{"user":"anna.lee","name":"Anna Lee"}]}`))
	body := RenderBody(d)
	for _, want := range []string{`Ask <span class="mention">@Anna Lee</span>!`, `<h1 id="anna-lee" data-block="h"><span class="mention">@Anna Lee</span></h1>`} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %s\n%s", want, body)
		}
	}
	hostile := Doc{Tabs: []Tab{{ID: "t", Title: "t", Blocks: []Block{{ID: "a", Type: Paragraph, Content: []Inline{{User: evil, Name: evil}}}}}}}
	page := RenderPage(hostile, "")
	for _, bad := range []string{"<script", "<img src=x", `onerror=alert(2)>`, `" onmouseover="`} {
		if strings.Contains(page, bad) {
			t.Errorf("rendered page contains live %q", bad)
		}
	}
	if strings.Contains(body, "<a ") || strings.Contains(body, "href") {
		t.Errorf("a mention is text, not a link: %s", body)
	}
}

func TestAMentionSurvivesMarkdownInEveryPlaceItCanAppear(t *testing.T) {
	t.Parallel()
	d := mustParse(t, one(`
{"id":"a","type":"paragraph","content":[{"text":"Ask "},`+anna+`,{"text":" to confirm"}]},
{"id":"b","type":"paragraph","content":[`+anna+`]},
{"id":"c","type":"heading","level":2,"content":[{"text":"For "},`+anna+`]},
{"id":"d","type":"bullet_list","items":[{"content":[`+anna+`,{"text":" owns it"}],"items":[{"content":[{"text":"cc "},{"user":"bob","name":"Bob [the] *Builder*"}]}]}]},
{"id":"e","type":"numbered_list","items":[{"content":[`+anna+`]}]},
{"id":"f","type":"task_list","items":[{"checked":true,"content":[{"text":"done by "},`+anna+`]}]},
{"id":"g","type":"table","header":true,"rows":[{"cells":[{"content":[{"text":"Owner"}]},{"content":[{"text":"Note"}]}]},{"cells":[{"content":[`+anna+`]},{"content":[{"text":"x | y "},{"user":"carol","name":"Carol | Co"}]}]}]},
{"id":"h","type":"quote","blocks":[{"id":"i","type":"paragraph","content":[`+anna+`]}]},
{"id":"j","type":"paragraph","content":[{"text":"bold","marks":["bold"]},{"text":" "},`+anna+`,{"text":" "},{"text":"link","marks":["link"],"href":"https://example.com"},{"user":"dave","name":"Dave"}]},
{"id":"k","type":"paragraph","content":[`+anna+`,`+anna+`]},
{"id":"m","type":"paragraph","content":[{"text":"wow!"},`+anna+`]},
{"id":"l","type":"paragraph","content":[{"user":"wang.xm","name":"王小明 😀"},{"text":"：好"}]}`))
	md := ToMarkdown(d)
	if !strings.Contains(md, "[@Anna Lee](mention:anna.lee)") {
		t.Errorf("the Markdown form is a link whose text is @name:\n%s", md)
	}
	first := fromMD(t, md, nil)
	if got, want := mentionsOf(first), mentionsOf(d); !reflect.DeepEqual(got, want) || len(want) != 16 {
		t.Errorf("the mentions did not come back\n got %v\nwant %v\nmarkdown:\n%s", got, want, md)
	}
	if err := mustValid(first); err != nil {
		t.Errorf("the converted doc does not validate: %v", err)
	}
	second := fromMD(t, ToMarkdown(first), &first)
	if !reflect.DeepEqual(first, second) {
		t.Errorf("a second round trip changed the blocks\n first %s\nsecond %s", canonicalOf(t, first), canonicalOf(t, second))
	}
}

// mentionsOf lists every mention of the doc, each with the text around it, in reading order.
func mentionsOf(d Doc) []string {
	var out []string
	runs := func(list []Inline) {
		for i, r := range list {
			if r.User == "" {
				continue
			}
			before, after := "", ""
			if i > 0 {
				before = list[i-1].Text
			}
			if i+1 < len(list) {
				after = list[i+1].Text
			}
			out = append(out, before+"<"+r.User+"="+r.Name+">"+after)
		}
	}
	var items func([]Item)
	items = func(list []Item) {
		for _, it := range list {
			runs(it.Content)
			items(it.Items)
		}
	}
	var walk func([]Block)
	walk = func(blocks []Block) {
		for _, b := range blocks {
			runs(b.Content)
			items(b.Items)
			for _, r := range b.Rows {
				for _, c := range r.Cells {
					runs(c.Content)
				}
			}
			walk(b.Blocks)
		}
	}
	for _, t := range d.Tabs {
		walk(t.Blocks)
	}
	return out
}

func TestAMarkdownReaderThatDoesNotKnowMentionsStillReadsAName(t *testing.T) {
	t.Parallel()
	md := ToMarkdown(mustParse(t, one(`{"id":"a","type":"paragraph","content":[{"text":"Ask "},`+anna+`]}`)))
	// What a reader without the mention rule shows is the link's text.
	if got := TextOfMarkdown(md); !strings.Contains(got, "Ask @Anna Lee") {
		t.Errorf("a plain Markdown reader shows %q", got)
	}
}

func TestAMentionLinkThatIsNotAMentionRefusesTheConversionAndNamesTheLine(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ md, want string }{
		"no at in the text":  {"ok\n\n[Anna](mention:anna)\n", "line 3"},
		"account with space": {"[@Anna](mention:a%20b)\n", "line 1"},
		"empty account":      {"[@Anna](mention:)\n", "line 1"},
		"empty name":         {"[@](mention:anna)\n", "line 1"},
	} {
		_, err := FromMarkdown(tc.md, nil, counting())
		switch {
		case err == nil:
			t.Errorf("%s: accepted", name)
		case !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "mention"):
			t.Errorf("%s: error %q should name %s and say mention", name, err, tc.want)
		}
	}
}

func TestAMentionInTheFirstHeadingIsPartOfTheTitleWithItsAt(t *testing.T) {
	t.Parallel()
	d := mustParse(t, one(`{"id":"h","type":"heading","level":1,"content":[{"text":"Plan for "},`+anna+`]}`))
	if got := Title(d); got != "Plan for @Anna Lee" {
		t.Errorf("title = %q", got)
	}
}

func canonicalOf(t *testing.T, d Doc) string {
	t.Helper()
	s, err := Canonical(d)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestMentionNamesKeepTheirLiteralCharactersThroughMarkdown(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"A & B", "<Anna>", "Anna \\ Lee", "Anna `code`", "Anna [x](https://example.com)", " Anna ", "王😀", "Anna ![] * _ ~ |"} {
		d := Doc{Tabs: []Tab{{ID: "t", Title: "Document", Blocks: []Block{{ID: "p", Type: Paragraph, Content: []Inline{{User: "anna", Name: name}}}}}}}
		converted := fromMD(t, ToMarkdown(d), nil)
		if got := mentionsOf(converted); !reflect.DeepEqual(got, mentionsOf(d)) {
			t.Errorf("%q through Markdown %q: got %v, want %v", name, ToMarkdown(d), got, mentionsOf(d))
		}
	}
}

func TestAssignedOIDCAccountsRemainMentionsAcrossBlocksAndMarkdown(t *testing.T) {
	t.Parallel()
	const user = "oidc:0123456789abcdef0123456789abcdef"
	d := mustParse(t, one(`{"id":"p","type":"paragraph","content":[{"text":"Ask "},{"user":"`+user+`","name":"OIDC colleague"}]}`))
	md := ToMarkdown(d)
	if !strings.Contains(md, "(mention:"+user+")") {
		t.Fatalf("external account lost in Markdown: %s", md)
	}
	roundTrip := fromMD(t, md, nil)
	if err := mustValid(roundTrip); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(mentionsOf(roundTrip), mentionsOf(d)) {
		t.Fatalf("identity changed in Markdown: %v vs %v", mentionsOf(roundTrip), mentionsOf(d))
	}
}

func TestOtherColonIdentifiersCannotBecomeMentions(t *testing.T) {
	t.Parallel()
	for _, user := range []string{"oidc:short", "oidc:" + strings.Repeat("a", 31), "oidc:" + strings.Repeat("a", 33), "oidc:" + strings.Repeat("A", 32), "OIDC:" + strings.Repeat("a", 32), "other:" + strings.Repeat("a", 32), "javascript:alert"} {
		body := one(`{"id":"p","type":"paragraph","content":[{"user":"` + user + `","name":"Colleague"}]}`)
		if err := Validate(body); err == nil {
			t.Errorf("accepted non-account %q in blocks", user)
		}
		if _, err := FromMarkdown("[@Colleague](mention:"+user+")", nil, counting()); err == nil {
			t.Errorf("accepted non-account %q in Markdown", user)
		}
	}
}
