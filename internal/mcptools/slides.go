package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/service"
)

const slideToolsShared = `
Slides are numbered from 1 in the order they are shown. The result is the deck's slide list after the change (number and title of
every slide, and overflows: true on a slide likely to overflow the canvas), the version that was saved, and review: what a publish of the deck would say (a slide likely to overflow, slides
without notes, a slide count that changed). Check the list matches what you meant.
The new deck is checked like a publish: an invalid slide is refused naming its place (for example "Slide 4 element 1 (table)"),
a slide number that does not exist is refused naming the numbers that do, and a refusal saves nothing.
baseVersion: send the version you read (get_slide, read, or the last slide tool's result). Slide numbers move when someone else
changes the deck, so a newer version refuses the call instead of changing the wrong slide; read again and retry.
Only the author and editors of the deck can change it, as with publish. The slide object is the one the slides guide in kinds
describes: {"title", "layout", "notes", "elements"}; send the object itself, not a string.`

const getSlideDoc = `Read one slide of a deck (an artifact of kind slides) by its number, with the list of all its slides (number and
title) and the version you read: use it before replace_slide, insert_slide, move_slide or delete_slide, so you change one slide
without resending the deck. read returns the whole deck. Anyone who can see the deck can read a slide.`

const replaceSlideDoc = `Replace one slide of a deck with a new slide object, leaving every other slide as it is: use it to change a slide
instead of publishing the whole deck again.` + slideToolsShared

const insertSlideDoc = `Insert a new slide into a deck after slide "after" (0 puts it first; the number of the last slide puts it at the
end); the new slide gets number after+1 and the slides after it move up by one.` + slideToolsShared

const moveSlideDoc = `Move one slide of a deck so that it gets the number "to"; the slides in between shift by one. The slide itself does
not change.` + slideToolsShared

const deleteSlideDoc = `Delete one slide of a deck; the slides after it move down by one. A deck keeps at least one slide. The deleted
slide stays in the earlier version (versions, restore).` + slideToolsShared

type getSlideIn struct {
	ID     string `json:"id" jsonschema:"the id of the deck"`
	Number int    `json:"number" jsonschema:"the number of the slide, from 1"`
}

type replaceSlideIn struct {
	ID          string `json:"id" jsonschema:"the id of the deck"`
	Number      int    `json:"number" jsonschema:"the number of the slide to replace, from 1"`
	Slide       any    `json:"slide" jsonschema:"the new slide: one slide object {title, layout, notes, elements}; send the object itself, not a string"`
	BaseVersion int    `json:"baseVersion,omitempty" jsonschema:"the version you read the slide numbers from; a newer one refuses the call"`
}

type insertSlideIn struct {
	ID          string `json:"id" jsonschema:"the id of the deck"`
	After       int    `json:"after" jsonschema:"the number of the slide the new one goes after; 0 puts it first"`
	Slide       any    `json:"slide" jsonschema:"the new slide: one slide object {title, layout, notes, elements}; send the object itself, not a string"`
	BaseVersion int    `json:"baseVersion,omitempty" jsonschema:"the version you read the slide numbers from; a newer one refuses the call"`
}

type moveSlideIn struct {
	ID          string `json:"id" jsonschema:"the id of the deck"`
	Number      int    `json:"number" jsonschema:"the number of the slide to move, from 1"`
	To          int    `json:"to" jsonschema:"the number the slide has after the move, from 1 to the number of slides"`
	BaseVersion int    `json:"baseVersion,omitempty" jsonschema:"the version you read the slide numbers from; a newer one refuses the call"`
}

type deleteSlideIn struct {
	ID          string `json:"id" jsonschema:"the id of the deck"`
	Number      int    `json:"number" jsonschema:"the number of the slide to delete, from 1"`
	BaseVersion int    `json:"baseVersion,omitempty" jsonschema:"the version you read the slide numbers from; a newer one refuses the call"`
}

type slidesEditedOut struct {
	service.SlidesEdited
	Next string `json:"next"`
}

type slideReadOut struct {
	Version int               `json:"version"`
	Number  int               `json:"number"`
	Slide   any               `json:"slide"`
	Slides  []kind.SlideEntry `json:"slides"`
}

func (t *tools) getSlide(ctx context.Context, _ *mcp.CallToolRequest, in getSlideIn) (*mcp.CallToolResult, slideReadOut, error) {
	res, err := t.e.svc.Slide(ctx, t.viewer(ctx), in.ID, in.Number)
	if err != nil {
		return nil, slideReadOut{}, t.fail("get_slide", err)
	}
	var slide any
	if err := json.Unmarshal(res.Slide, &slide); err != nil {
		return nil, slideReadOut{}, t.fail("get_slide", fmt.Errorf("slide %d of artifact %s: %w", in.Number, in.ID, err))
	}
	return nil, slideReadOut{Version: res.Version, Number: res.Number, Slide: slide, Slides: res.Slides}, nil
}

// editSlide runs one slide edit for the tool it is registered as.
func editSlide[In any](t *tools, tool string, edit func(In) (string, service.SlideEditInput, error)) mcp.ToolHandlerFor[In, slidesEditedOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, slidesEditedOut, error) {
		id, input, err := edit(in)
		if err != nil {
			return nil, slidesEditedOut{}, err
		}
		res, err := t.e.svc.EditSlide(t.asAgent(ctx), t.viewer(ctx), id, input)
		if err != nil {
			return nil, slidesEditedOut{}, t.fail(tool, err)
		}
		next := fmt.Sprintf("Saved as version %d; send baseVersion %d with the next slide tool.", res.Version, res.Version)
		if len(res.Review) > 0 {
			next += " Review before you tell the person it is done: see review."
		}
		return nil, slidesEditedOut{SlidesEdited: res, Next: next}, nil
	}
}

func slideArg(v any) ([]byte, error) {
	raw, err := jsonArg(v)
	if err != nil {
		return nil, errors.New("slide must be JSON: one slide object")
	}
	return raw, nil
}

// registerSlides adds the tools that read and change one slide of a deck.
func (t *tools) registerSlides(s *mcp.Server) {
	destructive := func(title string) *mcp.ToolAnnotations {
		return &mcp.ToolAnnotations{Title: title, DestructiveHint: ptr(true), OpenWorldHint: ptr(false)}
	}
	replace := editSlide(t, "replace_slide", func(in replaceSlideIn) (string, service.SlideEditInput, error) {
		raw, err := slideArg(in.Slide)
		return in.ID, service.SlideEditInput{Op: service.SlideReplace, Number: in.Number, Slide: raw, BaseVersion: in.BaseVersion}, err
	})
	insert := editSlide(t, "insert_slide", func(in insertSlideIn) (string, service.SlideEditInput, error) {
		raw, err := slideArg(in.Slide)
		return in.ID, service.SlideEditInput{Op: service.SlideInsert, After: in.After, Slide: raw, BaseVersion: in.BaseVersion}, err
	})
	move := editSlide(t, "move_slide", func(in moveSlideIn) (string, service.SlideEditInput, error) {
		return in.ID, service.SlideEditInput{Op: service.SlideMove, Number: in.Number, To: in.To, BaseVersion: in.BaseVersion}, nil
	})
	remove := editSlide(t, "delete_slide", func(in deleteSlideIn) (string, service.SlideEditInput, error) {
		return in.ID, service.SlideEditInput{Op: service.SlideDelete, Number: in.Number, BaseVersion: in.BaseVersion}, nil
	})
	mcp.AddTool(s, &mcp.Tool{Name: "get_slide", Description: getSlideDoc, Annotations: readOnly("Read one slide of a deck")}, t.getSlide)
	mcp.AddTool(s, &mcp.Tool{Name: "replace_slide", Description: replaceSlideDoc, Annotations: destructive("Replace one slide of a deck")}, replace)
	mcp.AddTool(s, &mcp.Tool{Name: "insert_slide", Description: insertSlideDoc, Annotations: destructive("Insert a slide into a deck")}, insert)
	mcp.AddTool(s, &mcp.Tool{Name: "move_slide", Description: moveSlideDoc, Annotations: destructive("Move one slide of a deck")}, move)
	mcp.AddTool(s, &mcp.Tool{Name: "delete_slide", Description: deleteSlideDoc, Annotations: destructive("Delete one slide of a deck")}, remove)
}
