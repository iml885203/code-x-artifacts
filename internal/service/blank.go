package service

import (
	"context"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/store"
)

// Blank kinds: what a person can start from the site without an agent.
const (
	blankDoc    = "doc"
	blankDesign = "design"
	blankBoard  = "board"
)

// blankBoardContent is a board with nothing on it.
const blankBoardContent = `{"version":1,"elements":[]}`

// blankDocBlocks is a doc that is one tab with one empty heading 1 to type in.
const blankDocBlocks = `[{"type":"heading","level":1}]`

// blankDesignContent is a design of one page with one frame, and blankFrame is
// the frame's file: a white page the size of a desktop screen, with nothing in it.
const (
	blankDesignContent = `{"version":1,"pages":[{"id":"page-1","name":"Page 1","rows":[[{"id":"frame-1","name":"Frame 1","file":"frames/frame-1.html","w":1440,"h":900}]]}]}`
	blankFrame         = `<!doctype html><html><head><meta charset="utf-8"><style>html,body{margin:0;height:100%}body{background:#fff}</style></head><body></body></html>`
)

// blankTitle is the title a blank artifact gets when the person gave none.
var blankTitle = map[string]string{blankDoc: "Untitled document", blankDesign: "Untitled design", blankBoard: "Untitled board"}

// CreateBlank makes the viewer's own empty doc, design or board, private, for the
// person to fill in from the site. title may be empty. Anything else a person
// publishes comes from an agent (the publish tool).
func (s *Service) CreateBlank(ctx context.Context, v Viewer, kind, title string) (Published, error) {
	if v.Anonymous() {
		return Published{}, refuse(Forbidden, "sign in to start something new")
	}
	in := PublishInput{Title: strings.TrimSpace(title), Kind: kind, Visibility: store.VisibilityPrivate}
	if in.Title == "" {
		in.Title = blankTitle[kind]
	}
	switch kind {
	case blankDoc:
		in.Blocks = []byte(blankDocBlocks)
	case blankDesign:
		frame := blankFrame
		in.Content, in.Files = blankDesignContent, map[string]*string{"frames/frame-1.html": &frame}
	case blankBoard:
		in.Content = blankBoardContent
	default:
		return Published{}, refuse(Invalid, `a new artifact can start as "doc", "design" or "board"; anything else is published by an agent`)
	}
	return s.Create(ctx, v, in)
}
