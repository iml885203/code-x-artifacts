package pptx

// Deck is what a PowerPoint file is written from: the slides of a deck as
// content, never as layout. The package lays them out.
type Deck struct {
	// Title is the file's title property.
	Title  string
	Slides []Slide
}

// Slide is one slide. A cover slide has its title large and centred with its
// elements under it; any other slide has its title at the top.
type Slide struct {
	Cover    bool
	Title    Text
	Notes    string
	Elements []Element
}

// Run is a stretch of text with one look.
type Run struct {
	Text string
	Bold bool
	Code bool
}

// Text is a line of runs; a newline in a run starts a new line.
type Text []Run

// Plain is text with no marks.
func Plain(s string) Text {
	if s == "" {
		return nil
	}
	return Text{{Text: s}}
}

// Element is one thing on a slide. Only the types of this package are elements.
type Element interface {
	// height is the room the element takes, in canvas pixels, laid out w
	// pixels wide with its type scaled by k.
	height(w, k float64) float64
	// draw adds the element's shapes to the slide in the box at x, y, w wide
	// and h high.
	draw(c *canvas, x, y, w, h, k float64)
}

// Paragraph is a block of text.
type Paragraph struct{ Text Text }

// List is a bulleted or numbered list.
type List struct {
	Ordered bool
	Items   []Text
}

// Code is preformatted text; Language is a label only.
type Code struct{ Code, Language string }

// Tone colours a table cell.
type Tone string

// The tones a cell can have.
const (
	ToneNone    Tone = ""
	ToneSuccess Tone = "success"
	ToneWarning Tone = "warning"
	ToneDanger  Tone = "danger"
)

// Cell is one cell of a table.
type Cell struct {
	Text Text
	Tone Tone
}

// Table is a header row and rows with one cell per column.
type Table struct {
	Columns []Text
	Rows    [][]Cell
}

// Stat is a big number with its label.
type Stat struct{ Value, Label Text }

// Stats is up to four big numbers side by side.
type Stats struct{ Items []Stat }

// Quote is a pull quote and who said it.
type Quote struct{ Text, By Text }

// Step is one step of a sequence.
type Step struct{ Title, Text Text }

// Steps is a numbered sequence across the slide.
type Steps struct{ Items []Step }

// Column is one column of a Columns element.
type Column struct {
	Title    Text
	Elements []Element
}

// Columns lays elements out side by side.
type Columns struct{ Columns []Column }

// Card is one card; at most one card of a set is highlighted.
type Card struct {
	Title, Text Text
	Items       []Text
	Highlight   bool
}

// Cards is a set of side-by-side cards.
type Cards struct{ Items []Card }

// Bar is one progress bar: Value is a percentage, Text replaces "NN%".
type Bar struct {
	Label, Text Text
	Value       float64
}

// Progress is a set of bars.
type Progress struct{ Items []Bar }
