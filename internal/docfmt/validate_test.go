package docfmt

import (
	"fmt"
	"strings"
	"testing"
)

// one wraps blocks (a JSON array body) in a doc with a single tab.
func one(blocks string) string {
	return `{"tabs":[{"id":"t1","title":"Main","blocks":[` + blocks + `]}]}`
}

func TestEveryBlockTypeAndMarkValidates(t *testing.T) {
	t.Parallel()
	marks := []string{MarkBold, MarkItalic, MarkUnderline, MarkStrike, MarkCode, MarkHighlight}
	var runs []string
	for _, m := range marks {
		runs = append(runs, fmt.Sprintf(`{"text":"x","marks":["%s"]}`, m))
	}
	runs = append(runs,
		`{"text":"site","marks":["link"],"href":"https://example.com/a?b=1&c=2"}`,
		`{"text":"web","marks":["link","bold"],"href":"http://example.com"}`,
		`{"text":"mail","marks":["link"],"href":"mailto:a@b.co"}`,
		`{"text":"up","marks":["link"],"href":"#goals"}`,
		`{"text":"two\nlines"}`)
	allMarks := strings.Join(runs, ",")
	for name, blocks := range map[string]string{
		"paragraph":     `{"id":"b1","type":"paragraph","content":[` + allMarks + `]}`,
		"empty para":    `{"id":"b1","type":"paragraph"}`,
		"heading":       `{"id":"b1","type":"heading","level":6,"content":[{"text":"H"}]}`,
		"bullet list":   `{"id":"b1","type":"bullet_list","items":[{"content":[{"text":"a"}],"items":[{"content":[{"text":"b"}]}]}]}`,
		"numbered list": `{"id":"b1","type":"numbered_list","start":3,"items":[{"content":[{"text":"a"}]}]}`,
		"task list":     `{"id":"b1","type":"task_list","items":[{"checked":true,"content":[{"text":"a"}]},{"checked":false}]}`,
		"quote":         `{"id":"b1","type":"quote","blocks":[{"id":"b2","type":"paragraph","content":[{"text":"q"}]},{"id":"b3","type":"bullet_list","items":[{"content":[{"text":"i"}]}]},{"id":"b4","type":"table","rows":[{"cells":[{}]}]},{"id":"b5","type":"image","src":"https://example.com/a.png"}]}`,
		"code":          `{"id":"b1","type":"code","lang":"c++","text":"int x;\n"}`,
		"code no lang":  `{"id":"b1","type":"code","text":""}`,
		"rule":          `{"id":"b1","type":"rule"}`,
		"table":         `{"id":"b1","type":"table","header":true,"rows":[{"cells":[{"align":"left","content":[{"text":"a"}]},{"align":"right"}]},{"cells":[{"align":"center"},{}]}]}`,
		"image https":   `{"id":"b1","type":"image","src":"https://example.com/a.png","alt":"a"}`,
		"image file":    `{"id":"b1","type":"image","src":"/files/AbC123/chart.png","alt":""}`,
		"image data":    `{"id":"b1","type":"image","src":"data:image/png;base64,iVBORw0KGgo=","alt":"dot"}`,
		"pending":       `{"id":"b1","type":"pending","intent":"Goals: the three outcomes"}`,
	} {
		if err := Validate(one(blocks)); err != nil {
			t.Errorf("%s: refused: %v", name, err)
		}
	}
	if err := Validate(`{"tabs":[{"id":"t1","title":"A","blocks":[]},{"id":"t2","title":"B","children":[],"blocks":[{"id":"b1","type":"rule"}]}]}`); err != nil {
		t.Errorf("two tabs: refused: %v", err)
	}
}

func TestEveryInvalidDocIsRefusedWithThePathOfTheProblem(t *testing.T) {
	t.Parallel()
	const bad = `"id":"b1","type":"paragraph"`
	for name, tc := range map[string]struct{ doc, want string }{
		"not json":               {`tabs`, "JSON"},
		"trailing data":          {one(``) + `{}`, "JSON"},
		"not an object":          {`[]`, "document: must be an object"},
		"unknown top field":      {`{"tabs":[],"x":1}`, `document: unknown field "x"`},
		"no tabs":                {`{"tabs":[]}`, "tabs: a doc needs at least one tab"},
		"tabs not a list":        {`{"tabs":{}}`, "tabs: must be a list"},
		"tab without id":         {`{"tabs":[{"title":"a","blocks":[]}]}`, "tabs[0].id: required"},
		"tab title two lines":    {`{"tabs":[{"id":"t","title":"a\nb","blocks":[]}]}`, "tabs[0].title: must be one line"},
		"tab title comment end":  {`{"tabs":[{"id":"t","title":"a --> b","blocks":[]}]}`, "tabs[0].title: must not contain -->"},
		"tab without title":      {`{"tabs":[{"id":"t","title":"","blocks":[]}]}`, "tabs[0].title: must not be empty"},
		"children used":          {`{"tabs":[{"id":"t","title":"a","children":[{}],"blocks":[]}]}`, "tabs[0].children: nested tabs are not supported yet"},
		"unknown tab field":      {`{"tabs":[{"id":"t","title":"a","blocks":[],"color":"red"}]}`, `tabs[0]: unknown field "color"`},
		"block without id":       {one(`{"type":"rule"}`), "tabs[0].blocks[0].id: required"},
		"bad id":                 {one(`{"id":"a b","type":"rule"}`), "tabs[0].blocks[0].id"},
		"long id":                {one(`{"id":"` + strings.Repeat("a", MaxIDLength+1) + `","type":"rule"}`), "tabs[0].blocks[0].id"},
		"duplicate id":           {one(`{"id":"b1","type":"rule"},{"id":"b1","type":"rule"}`), `tabs[0].blocks[1].id: "b1" is already used by tabs[0].blocks[0]`},
		"tab and block same id":  {one(`{"id":"t1","type":"rule"}`), `tabs[0].blocks[0].id: "t1" is already used by tabs[0]`},
		"unknown type":           {one(`{"id":"b1","type":"video"}`), `tabs[0].blocks[0].type: unknown block type "video"`},
		"no type":                {one(`{"id":"b1"}`), "tabs[0].blocks[0].type: required"},
		"field of another type":  {one(`{"id":"b1","type":"rule","text":"x"}`), `tabs[0].blocks[0]: unknown field "text"`},
		"heading level 0":        {one(`{"id":"b1","type":"heading","level":0}`), "tabs[0].blocks[0].level: must be 1 to 6"},
		"heading level 7":        {one(`{"id":"b1","type":"heading","level":7}`), "tabs[0].blocks[0].level: must be 1 to 6"},
		"heading level text":     {one(`{"id":"b1","type":"heading","level":"2"}`), "tabs[0].blocks[0].level: must be 1 to 6, a whole number"},
		"heading level fraction": {one(`{"id":"b1","type":"heading","level":2.5}`), "tabs[0].blocks[0].level: must be 1 to 6, a whole number"},
		"run without text":       {one(`{` + bad + `,"content":[{"marks":["bold"]}]}`), "tabs[0].blocks[0].content[0].text: required"},
		"empty run":              {one(`{` + bad + `,"content":[{"text":""}]}`), "tabs[0].blocks[0].content[0].text: must not be empty"},
		"nul in text":            {one(`{` + bad + `,"content":[{"text":"a\u0000b"}]}`), "tabs[0].blocks[0].content[0].text: must not contain a NUL"},
		"unknown mark":           {one(`{` + bad + `,"content":[{"text":"a","marks":["sparkle"]}]}`), `tabs[0].blocks[0].content[0].marks[0]: unknown mark "sparkle"`},
		"repeated mark":          {one(`{` + bad + `,"content":[{"text":"a","marks":["bold","bold"]}]}`), "tabs[0].blocks[0].content[0].marks[1]: repeats"},
		"link without href":      {one(`{` + bad + `,"content":[{"text":"a","marks":["link"]}]}`), "tabs[0].blocks[0].content[0].href: a link needs an address"},
		"href without link":      {one(`{` + bad + `,"content":[{"text":"a","href":"https://x.co"}]}`), "tabs[0].blocks[0].content[0].href: only a run with the link mark has one"},
		"javascript link":        {one(`{` + bad + `,"content":[{"text":"a","marks":["link"],"href":"javascript:alert(1)"}]}`), "tabs[0].blocks[0].content[0].href: the scheme javascript: is not allowed"},
		"mixed case javascript":  {one(`{` + bad + `,"content":[{"text":"a","marks":["link"],"href":"JaVaScRiPt:alert(1)"}]}`), "is not allowed"},
		"data link":              {one(`{` + bad + `,"content":[{"text":"a","marks":["link"],"href":"data:text/html,x"}]}`), "is not allowed"},
		"relative link":          {one(`{` + bad + `,"content":[{"text":"a","marks":["link"],"href":"/x"}]}`), "must start with https:, http:, mailto: or #"},
		"link with space":        {one(`{` + bad + `,"content":[{"text":"a","marks":["link"],"href":"https://a b"}]}`), "href"},
		"link without host":      {one(`{` + bad + `,"content":[{"text":"a","marks":["link"],"href":"https://"}]}`), "href"},
		"link too long":          {one(`{` + bad + `,"content":[{"text":"a","marks":["link"],"href":"https://a.co/` + strings.Repeat("x", MaxLinkBytes) + `"}]}`), "at most 2048"},
		"list item field":        {one(`{"id":"b1","type":"bullet_list","items":[{"checked":true}]}`), "tabs[0].blocks[0].items[0].checked: only a task list has it"},
		"task item unchecked":    {one(`{"id":"b1","type":"task_list","items":[{"content":[{"text":"a"}]}]}`), "tabs[0].blocks[0].items[0].checked: required"},
		"empty list":             {one(`{"id":"b1","type":"bullet_list","items":[]}`), "tabs[0].blocks[0].items: a list needs at least one item"},
		"nested item bad":        {one(`{"id":"b1","type":"bullet_list","items":[{"items":[{"content":[{"text":"a","marks":["x"]}]}]}]}`), "tabs[0].blocks[0].items[0].items[0].content[0].marks[0]"},
		"numbered start":         {one(`{"id":"b1","type":"numbered_list","start":-1,"items":[{}]}`), "tabs[0].blocks[0].start: must be 0 or more"},
		"start on bullets":       {one(`{"id":"b1","type":"bullet_list","start":2,"items":[{}]}`), `unknown field "start"`},
		"quote holds pending":    {one(`{"id":"b1","type":"quote","blocks":[{"id":"b2","type":"pending","intent":"x"}]}`), "a quote cannot hold a pending"},
		"empty quote":            {one(`{"id":"b1","type":"quote","blocks":[]}`), "tabs[0].blocks[0].blocks: a quote needs at least one block"},
		"code text number":       {one(`{"id":"b1","type":"code","text":5}`), "tabs[0].blocks[0].text: must be text"},
		"code bad lang":          {one(`{"id":"b1","type":"code","lang":"a b","text":""}`), "tabs[0].blocks[0].lang"},
		"table no rows":          {one(`{"id":"b1","type":"table","rows":[]}`), "tabs[0].blocks[0].rows: a table needs at least one row"},
		"table empty row":        {one(`{"id":"b1","type":"table","rows":[{"cells":[]}]}`), "tabs[0].blocks[0].rows[0].cells: a row needs at least one cell"},
		"table ragged":           {one(`{"id":"b1","type":"table","rows":[{"cells":[{},{}]},{"cells":[{}]}]}`), "tabs[0].blocks[0].rows[1].cells: has 1 cells, the first row has 2"},
		"table bad align":        {one(`{"id":"b1","type":"table","rows":[{"cells":[{"align":"middle"}]}]}`), "tabs[0].blocks[0].rows[0].cells[0].align: must be left, center or right"},
		"table cell field":       {one(`{"id":"b1","type":"table","rows":[{"cells":[{"colspan":2}]}]}`), `tabs[0].blocks[0].rows[0].cells[0]: unknown field "colspan"`},
		"image no src":           {one(`{"id":"b1","type":"image","alt":"a"}`), "tabs[0].blocks[0].src: required"},
		"image http":             {one(`{"id":"b1","type":"image","src":"http://example.com/a.png"}`), "tabs[0].blocks[0].src: must be"},
		"image javascript":       {one(`{"id":"b1","type":"image","src":"javascript:alert(1)"}`), "tabs[0].blocks[0].src: must be"},
		"image svg data":         {one(`{"id":"b1","type":"image","src":"data:image/svg+xml;base64,PHN2Zz4="}`), "tabs[0].blocks[0].src: must be"},
		"image data too big":     {one(`{"id":"b1","type":"image","src":"data:image/png;base64,` + strings.Repeat("A", MaxDataURIBytes) + `"}`), "at most 262144 bytes"},
		"image relative":         {one(`{"id":"b1","type":"image","src":"pic.png"}`), "tabs[0].blocks[0].src: must be"},
		"pending no intent":      {one(`{"id":"b1","type":"pending"}`), "tabs[0].blocks[0].intent: required"},
		"pending newline":        {one(`{"id":"b1","type":"pending","intent":"a\nb"}`), "tabs[0].blocks[0].intent: must be one line"},
		"pending comment close":  {one(`{"id":"b1","type":"pending","intent":"a --> b"}`), "tabs[0].blocks[0].intent: must not contain -->"},
	} {
		err := Validate(tc.doc)
		switch {
		case err == nil:
			t.Errorf("%s: accepted, want a refusal mentioning %q", name, tc.want)
		case !strings.Contains(err.Error(), tc.want):
			t.Errorf("%s: error %q does not mention %q", name, err, tc.want)
		}
	}
}

func TestTheLimitsNameWhichLimitAndTheCurrentValue(t *testing.T) {
	t.Parallel()
	var many strings.Builder
	for i := 0; i <= MaxBlocks; i++ {
		fmt.Fprintf(&many, `{"id":"b%d","type":"rule"},`, i)
	}
	var tabs strings.Builder
	for i := 0; i <= MaxTabs; i++ {
		fmt.Fprintf(&tabs, `{"id":"t%d","title":"x","blocks":[]},`, i)
	}
	var cells strings.Builder
	for i := 0; i <= MaxTableCells/10; i++ {
		cells.WriteString(`{"cells":[{},{},{},{},{},{},{},{},{},{}]},`)
	}
	var wide strings.Builder
	for i := 0; i <= MaxTableColumns; i++ {
		wide.WriteString(`{},`)
	}
	for name, tc := range map[string]struct{ doc, want string }{
		"blocks":  {one(strings.TrimSuffix(many.String(), ",")), fmt.Sprintf("this doc has %d blocks; at most %d are allowed", MaxBlocks+1, MaxBlocks)},
		"depth":   {one(nest(MaxDepth)), fmt.Sprintf("nested %d levels deep; at most %d are allowed", MaxDepth+1, MaxDepth)},
		"tabs":    {`{"tabs":[` + strings.TrimSuffix(tabs.String(), ",") + `]}`, fmt.Sprintf("this doc has %d tabs; at most %d are allowed", MaxTabs+1, MaxTabs)},
		"cells":   {one(`{"id":"b1","type":"table","rows":[` + strings.TrimSuffix(cells.String(), ",") + `]}`), fmt.Sprintf("this table has %d cells; at most %d are allowed", (MaxTableCells/10+1)*10, MaxTableCells)},
		"columns": {one(`{"id":"b1","type":"table","rows":[{"cells":[` + strings.TrimSuffix(wide.String(), ",") + `]}]}`), fmt.Sprintf("this table has %d columns; at most %d are allowed", MaxTableColumns+1, MaxTableColumns)},
		"bytes":   {one(`{"id":"b1","type":"code","text":"` + strings.Repeat("a", MaxBytes) + `"}`), fmt.Sprintf("at most %d bytes", MaxBytes)},
		"title":   {`{"tabs":[{"id":"t","title":"` + strings.Repeat("a", MaxTitleRunes+1) + `","blocks":[]}]}`, fmt.Sprintf("is %d characters; at most %d", MaxTitleRunes+1, MaxTitleRunes)},
	} {
		err := Validate(tc.doc)
		switch {
		case err == nil:
			t.Errorf("%s: accepted", name)
		case !strings.Contains(err.Error(), tc.want):
			t.Errorf("%s: error %q does not mention %q", name, trim(err.Error()), tc.want)
		}
	}
}

func trim(s string) string {
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}

func TestADocAtEveryLimitStillValidates(t *testing.T) {
	t.Parallel()
	var many strings.Builder
	for i := 0; i < MaxBlocks; i++ {
		if i > 0 {
			many.WriteString(",")
		}
		fmt.Fprintf(&many, `{"id":"b%d","type":"rule"}`, i)
	}
	if err := Validate(one(many.String())); err != nil {
		t.Errorf("%d blocks refused: %v", MaxBlocks, err)
	}
	if err := Validate(one(nest(MaxDepth - 1))); err != nil {
		t.Errorf("%d levels refused: %v", MaxDepth, err)
	}
}

// nest is a paragraph inside n quotes: it sits n+1 levels deep.
func nest(n int) string {
	block := `{"id":"p","type":"paragraph"}`
	for i := 0; i < n; i++ {
		block = fmt.Sprintf(`{"id":"q%d","type":"quote","blocks":[%s]}`, i, block)
	}
	return block
}
