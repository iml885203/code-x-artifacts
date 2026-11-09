package kind

import (
	"encoding/json"
	"strings"
	"testing"
)

func titlesOf(t *testing.T, d Deck) string {
	t.Helper()
	var titles []string
	for i, s := range d.List() {
		if s.Number != i+1 {
			t.Fatalf("slide %d is numbered %d", i+1, s.Number)
		}
		titles = append(titles, s.Title)
	}
	return strings.Join(titles, ",")
}

func rawSlide(t *testing.T, title string) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(sl(title))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestEachSlideOperationGivesTheExpectedDeckAndLeavesTheOriginalAlone(t *testing.T) {
	src := deck(t, sl("A"), sl("B"), sl("C"))
	d, err := ParseDeck(src, slidesFormatV1)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		do   func(Deck) (Deck, error)
		want string
	}{
		{"replace the second", func(d Deck) (Deck, error) { return d.Replace(2, rawSlide(t, "X")) }, "A,X,C"},
		{"insert at the start", func(d Deck) (Deck, error) { return d.InsertAfter(0, rawSlide(t, "X")) }, "X,A,B,C"},
		{"insert after the first", func(d Deck) (Deck, error) { return d.InsertAfter(1, rawSlide(t, "X")) }, "A,X,B,C"},
		{"insert after the last", func(d Deck) (Deck, error) { return d.InsertAfter(3, rawSlide(t, "Z")) }, "A,B,C,Z"},
		{"move the first to the end", func(d Deck) (Deck, error) { return d.Move(1, 3) }, "B,C,A"},
		{"move the last to the start", func(d Deck) (Deck, error) { return d.Move(3, 1) }, "C,A,B"},
		{"move a slide to where it is", func(d Deck) (Deck, error) { return d.Move(2, 2) }, "A,B,C"},
		{"delete the middle", func(d Deck) (Deck, error) { return d.Delete(2) }, "A,C"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := c.do(d)
			if err != nil {
				t.Fatal(err)
			}
			if titles := titlesOf(t, got); titles != c.want {
				t.Errorf("slides %s, want %s", titles, c.want)
			}
			if err := (Slides{}).Validate(got.Content(), slidesFormatV1); err != nil {
				t.Errorf("the edited deck is not valid: %v", err)
			}
			again, err := ParseDeck(got.Content(), slidesFormatV1)
			if err != nil || titlesOf(t, again) != c.want {
				t.Errorf("the content does not read back as the same deck: %v", err)
			}
			if titlesOf(t, d) != "A,B,C" {
				t.Errorf("the operation changed the deck it started from: %s", titlesOf(t, d))
			}
		})
	}
}

func TestAnEditKeepsEverySlideItDidNotTouch(t *testing.T) {
	src := deck(t, sl("A", everyElement()["table"]), sl("B"), sl("C", everyElement()["cards"]))
	d, err := ParseDeck(src, slidesFormatV1)
	if err != nil {
		t.Fatal(err)
	}
	got, err := d.Replace(2, rawSlide(t, "X"))
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{1, 3} {
		before, _ := d.Slide(n)
		after, _ := got.Slide(n)
		var x, y any
		if json.Unmarshal(before, &x) != nil || json.Unmarshal(after, &y) != nil || string(mustMarshal(t, x)) != string(mustMarshal(t, y)) {
			t.Errorf("slide %d changed:\n%s\n%s", n, before, after)
		}
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestASlideNumberOutOfRangeIsRefusedWithTheValidRange(t *testing.T) {
	d, err := ParseDeck(deck(t, sl("A"), sl("B"), sl("C")), slidesFormatV1)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		do   func() error
		want string
	}{
		{"get 4", func() error { _, err := d.Slide(4); return err }, "slides 1 to 3"},
		{"get 0", func() error { _, err := d.Slide(0); return err }, "slides 1 to 3"},
		{"replace 9", func() error { _, err := d.Replace(9, rawSlide(t, "X")); return err }, "slides 1 to 3"},
		{"delete -1", func() error { _, err := d.Delete(-1); return err }, "slides 1 to 3"},
		{"move from 4", func() error { _, err := d.Move(4, 1); return err }, "slides 1 to 3"},
		{"move to 4", func() error { _, err := d.Move(1, 4); return err }, "1 to 3"},
		{"insert after 4", func() error { _, err := d.InsertAfter(4, rawSlide(t, "X")); return err }, "0 to 3"},
		{"insert after -1", func() error { _, err := d.InsertAfter(-1, rawSlide(t, "X")); return err }, "0 to 3"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.do()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %v, want a refusal naming %q", err, c.want)
			}
		})
	}
}

func TestASlideEditRefusesWhatCannotBeASlideAndTheLastSlide(t *testing.T) {
	d, err := ParseDeck(deck(t, sl("A")), slidesFormatV1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Delete(1); err == nil || !strings.Contains(err.Error(), "at least one slide") {
		t.Errorf("deleting the only slide: %v", err)
	}
	for name, raw := range map[string]string{"nothing": "", "null": "null", "a string": `"{\"title\":\"X\"}"`} {
		if _, err := d.Replace(1, json.RawMessage(raw)); err == nil || !strings.Contains(err.Error(), "slide object") {
			t.Errorf("replace with %s: %v", name, err)
		}
		if _, err := d.InsertAfter(1, json.RawMessage(raw)); err == nil || !strings.Contains(err.Error(), "slide object") {
			t.Errorf("insert of %s: %v", name, err)
		}
	}
	if _, err := ParseDeck(deck(t, sl("A")), "2"); err == nil {
		t.Error("a deck of a format this build does not edit was taken")
	}
}
