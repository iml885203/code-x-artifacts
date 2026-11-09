package kind

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Deck is a saved slides artifact taken apart into its slides, for changing one
// slide without the publisher resending the others. Every operation returns a
// new Deck and leaves the one it was called on as it was; none of them
// validates the slide it is given, because the content of the result goes
// through Validate like any publish.
type Deck struct {
	slides []json.RawMessage
}

// SlideEntry is one slide as a slide edit lists the deck: its number from 1,
// its title, and whether it is likely to overflow the canvas (the same
// estimate as the review), so the editor can say so on the slide.
type SlideEntry struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Overflows bool   `json:"overflows,omitempty"`
}

// ErrDeckFormat is the refusal of a deck whose format the slide edits do not
// know.
var ErrDeckFormat = errors.New("slide edits do not know this format")

// ParseDeck reads a deck that was saved, so it is valid; an error means the
// stored content is not a deck of a format this build edits.
func ParseDeck(content, format string) (Deck, error) {
	if format != slidesFormatV1 {
		return Deck{}, fmt.Errorf("%w: this deck is format %q and slide edits know format %q; publish the whole deck instead", ErrDeckFormat, format, slidesFormatV1)
	}
	var top struct {
		Slides []json.RawMessage `json:"slides"`
	}
	if err := json.Unmarshal([]byte(content), &top); err != nil {
		return Deck{}, fmt.Errorf("the deck does not parse: %w", err)
	}
	return Deck{slides: top.Slides}, nil
}

// List numbers the slides and names each by its title.
func (d Deck) List() []SlideEntry {
	out := make([]SlideEntry, len(d.slides))
	for i, raw := range d.slides {
		var s struct {
			Title string `json:"title"`
		}
		_ = json.Unmarshal(raw, &s) // a slide without a readable title is listed with an empty one
		out[i] = SlideEntry{Number: i + 1, Title: s.Title, Overflows: overflows(i+1, raw)}
	}
	return out
}

// overflows is the review's estimate for one stored slide: valid, and likely
// to overflow the canvas even at the smallest type.
func overflows(n int, raw json.RawMessage) bool {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return false
	}
	s, problems := parseSlide(n, v)
	if len(problems) > 0 {
		return false
	}
	_, fits, _, _ := s.fit()
	return !fits
}

// Slide is slide n as it is stored.
func (d Deck) Slide(n int) (json.RawMessage, error) {
	if err := d.exists(n); err != nil {
		return nil, err
	}
	return d.slides[n-1], nil
}

// Replace puts s in the place of slide n.
func (d Deck) Replace(n int, s json.RawMessage) (Deck, error) {
	if err := d.exists(n); err != nil {
		return Deck{}, err
	}
	if err := slideObject(s); err != nil {
		return Deck{}, err
	}
	out := d.clone()
	out.slides[n-1] = s
	return out, nil
}

// InsertAfter puts s after slide n, so it becomes slide n+1; after 0 is the
// start of the deck.
func (d Deck) InsertAfter(n int, s json.RawMessage) (Deck, error) {
	if n < 0 || n > len(d.slides) {
		return Deck{}, fmt.Errorf("after must be from 0 to %[1]d (0 puts the slide first, %[1]d after the last slide); got %[2]d", len(d.slides), n)
	}
	if err := slideObject(s); err != nil {
		return Deck{}, err
	}
	out := make([]json.RawMessage, 0, len(d.slides)+1)
	out = append(out, d.slides[:n]...)
	out = append(out, s)
	out = append(out, d.slides[n:]...)
	return Deck{slides: out}, nil
}

// Move takes slide n out and puts it back so that it is slide to.
func (d Deck) Move(n, to int) (Deck, error) {
	if err := d.exists(n); err != nil {
		return Deck{}, err
	}
	if to < 1 || to > len(d.slides) {
		return Deck{}, fmt.Errorf("to is the number the slide will have, from 1 to %d; got %d", len(d.slides), to)
	}
	out := make([]json.RawMessage, 0, len(d.slides))
	out = append(out, d.slides[:n-1]...)
	out = append(out, d.slides[n:]...)
	out = append(out[:to-1], append([]json.RawMessage{d.slides[n-1]}, out[to-1:]...)...)
	return Deck{slides: out}, nil
}

// Delete removes slide n. A deck keeps at least one slide.
func (d Deck) Delete(n int) (Deck, error) {
	if err := d.exists(n); err != nil {
		return Deck{}, err
	}
	if len(d.slides) == 1 {
		return Deck{}, errors.New("a deck has at least one slide and this is its only one: replace it instead, or delete the artifact")
	}
	out := make([]json.RawMessage, 0, len(d.slides)-1)
	out = append(out, d.slides[:n-1]...)
	out = append(out, d.slides[n:]...)
	return Deck{slides: out}, nil
}

// Content is the deck as stored: one slide per line, so a reader of the JSON
// and a diff of two versions see slides, not one long line.
func (d Deck) Content() string {
	lines := make([]string, len(d.slides))
	for i, raw := range d.slides {
		var b bytes.Buffer
		if json.Compact(&b, raw) != nil {
			b.Reset()
			b.Write(raw)
		}
		lines[i] = b.String()
	}
	return "{\"version\":1,\"slides\":[\n" + strings.Join(lines, ",\n") + "\n]}"
}

func (d Deck) exists(n int) error {
	if n < 1 || n > len(d.slides) {
		if len(d.slides) == 1 {
			return fmt.Errorf("there is no slide %d: this deck has one slide, number 1", n)
		}
		return fmt.Errorf("there is no slide %d: this deck has slides 1 to %d", n, len(d.slides))
	}
	return nil
}

func (d Deck) clone() Deck {
	return Deck{slides: append([]json.RawMessage(nil), d.slides...)}
}

// slideObject refuses what cannot be a slide before it is put in a deck, where
// it would read as a problem with the deck rather than with what was sent.
func slideObject(s json.RawMessage) error {
	trimmed := bytes.TrimSpace(s)
	switch {
	case len(trimmed) == 0 || string(trimmed) == "null":
		return errors.New(`slide is missing: send one slide object, {"title", "layout", "notes", "elements"}, as the kinds guide for slides describes`)
	case trimmed[0] == '"':
		return errors.New("slide is a string: send the slide object itself, not JSON inside a string")
	case trimmed[0] != '{':
		return errors.New(`slide must be one slide object, {"title", "layout", "notes", "elements"}`)
	}
	return nil
}
