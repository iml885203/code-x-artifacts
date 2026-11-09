// Package docfmt is the structured content of a doc artifact, format "2": tabs
// of blocks with stable ids, as JSON. It holds the model, the validation that
// every doc passes before it is stored, and the renderer that draws a doc as a
// read-only page (docs/doc-format.md). Markdown conversion lives here too.
//
// The model is pictor's own schema, shaped so a ProseMirror editor can map to
// and from it; it never stores an editor's internal JSON.
package docfmt

// Block types.
const (
	Paragraph    = "paragraph"
	Heading      = "heading"
	BulletList   = "bullet_list"
	NumberedList = "numbered_list"
	TaskList     = "task_list"
	Quote        = "quote"
	Code         = "code"
	Rule         = "rule"
	Table        = "table"
	Image        = "image"
	Pending      = "pending"
)

// Marks an inline run can carry.
const (
	MarkBold      = "bold"
	MarkItalic    = "italic"
	MarkUnderline = "underline"
	MarkStrike    = "strike"
	MarkCode      = "code"
	MarkHighlight = "highlight"
	MarkLink      = "link"
)

// Doc is a whole document.
type Doc struct {
	Tabs []Tab `json:"tabs"`
}

// Tab is one page of the document. Children is reserved for nested tabs: it
// must be empty today.
type Tab struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Children []Tab   `json:"children,omitempty"`
	Blocks   []Block `json:"blocks"`
}

// Block is one block of a tab; Type says which of the other fields it uses.
type Block struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	// Level is a heading's level, 1 to 6.
	Level int `json:"level,omitempty"`
	// Content is the text of a paragraph or a heading.
	Content []Inline `json:"content,omitempty"`
	// Blocks are what a quote holds.
	Blocks []Block `json:"blocks,omitempty"`
	// Items are the items of a list; Start is the first number of a numbered
	// list when it is not 1.
	Items []Item `json:"items,omitempty"`
	Start int    `json:"start,omitempty"`
	// Lang and Text are a code block's language and source.
	Lang string `json:"lang,omitempty"`
	Text string `json:"text,omitempty"`
	// Header says the first row of a table is its header; Rows are the rows.
	Header bool  `json:"header,omitempty"`
	Rows   []Row `json:"rows,omitempty"`
	// Src and Alt are an image's address and description; Size is one of
	// ImageSizes, or empty for the picture's own width (at most the page's).
	Src  string `json:"src,omitempty"`
	Alt  string `json:"alt,omitempty"`
	Size string `json:"size,omitempty"`
	// Intent says what a pending block's content will be.
	Intent string `json:"intent,omitempty"`
}

// Item is one item of a list. Checked is set on task list items only.
type Item struct {
	Content []Inline `json:"content,omitempty"`
	Checked *bool    `json:"checked,omitempty"`
	Items   []Item   `json:"items,omitempty"`
}

// Row is a row of a table.
type Row struct {
	Cells []Cell `json:"cells"`
}

// Cell is a table cell. Align is "", "left", "center" or "right".
type Cell struct {
	Align   string   `json:"align,omitempty"`
	Content []Inline `json:"content,omitempty"`
}

// Inline is a run of text with the same marks, a mention, or a date. Href is the
// address of a run with the link mark and only of those. A newline in Text is a
// line break. A mention is a run with User (the account) and Name (the name it
// had when it was mentioned) and nothing else: no text, no marks, no address.
// A date holds only Date, an ISO calendar day without a time or timezone.
type Inline struct {
	Dropdown *Dropdown `json:"dropdown,omitempty"`
	Date     string    `json:"date,omitempty"`
	Text     string    `json:"text,omitempty"`
	Marks    []string  `json:"marks,omitempty"`
	Href     string    `json:"href,omitempty"`
	User     string    `json:"user,omitempty"`
	Name     string    `json:"name,omitempty"`
}

// MentionScheme starts the address of a mention in Markdown:
// [@Anna Lee](mention:anna.lee).
const MentionScheme = "mention:"

// Mention reports whether the run is a mention of a person.
func (in Inline) Mention() bool { return in.User != "" }

// ImageSizes are the widths an image can be shown at, in CSS pixels.
var ImageSizes = map[string]int{"small": 224, "medium": 336, "large": 504, "xlarge": 672}

// Limits. A refusal names the limit and the value that went over it.
const (
	MaxBytes        = 2 << 20
	MaxTabs         = 50
	MaxBlocks       = 5000
	MaxDepth        = 8
	MaxListItems    = 5000
	MaxTableCells   = 2000
	MaxTableColumns = 30
	MaxIDLength     = 64
	MaxTitleRunes   = 200
	MaxLinkBytes    = 2048
	MaxDataURIBytes = 256 << 10
	MaxIntentRunes  = 500
	MaxAltRunes     = 1000
	MaxLangRunes    = 40
	// MaxMentionUserRunes and MaxMentionNameRunes bound a mention's account and name.
	MaxMentionUserRunes = 64
	MaxMentionNameRunes = 100
)
