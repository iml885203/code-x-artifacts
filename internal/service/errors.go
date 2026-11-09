package service

import (
	"errors"
	"fmt"

	"github.com/iml885203/code-x-artifacts/internal/boardedit"
	"github.com/iml885203/code-x-artifacts/internal/docedit"
	"github.com/iml885203/code-x-artifacts/internal/store"
)

// Code classifies a refusal so each front end can answer in its own way
// (an HTTP status, an MCP tool error).
type Code int

const (
	// NotFound means there is no such thing, or the viewer may not know there is.
	NotFound Code = iota + 1
	// Forbidden means the viewer can see the thing but may not change it.
	Forbidden
	// Invalid means the request itself is wrong; the message says what to send.
	Invalid
	// Conflict means the request was built on a version that is no longer the
	// newest; Error.Current says which is, and nothing was changed.
	Conflict
	// Busy means the server holds as much of this as it may right now; the
	// same request can succeed later, and nothing was changed.
	Busy
)

// Error is a refusal meant for the caller. Its message is shown to the person
// or agent, so it says what was wrong and what to do instead. Anything that is
// not an *Error is an internal failure and is never shown.
type Error struct {
	Code Code
	Msg  string
	// What names the thing a NotFound is about ("artifact" or "document") for
	// front ends that can say more than the bare message. Not part of the
	// HTTP answer.
	What string
	// Current is the newest version number, set on a Conflict.
	Current int
	// Latest is the newest version itself, set on a Conflict, so the caller
	// can merge without asking again.
	Latest *store.Artifact
	// ArtifactID names the artifact a Conflict is about.
	ArtifactID string
	// Doc is set on a Conflict about a shared document that was written
	// against a version it no longer has.
	Doc *DocConflict
	// Stale is set on a Conflict about blocks of a doc that are not what the
	// caller read: each as it is now.
	Stale []docedit.State
	// StaleElements is set on a Conflict about elements of a board that are not
	// what the caller read: each as it is now, with its hash.
	StaleElements map[string]boardedit.Current
}

func (e *Error) Error() string { return e.Msg }

func refuse(c Code, format string, args ...any) error {
	return &Error{Code: c, Msg: fmt.Sprintf(format, args...)}
}

// CodeOf returns the refusal code of err, or 0 when err is not a refusal.
func CodeOf(err error) Code {
	if e, ok := errors.AsType[*Error](err); ok {
		return e.Code
	}
	return 0
}

var (
	errNotFound   = &Error{Code: NotFound, Msg: "not found", What: "artifact"}
	errNoDocument = &Error{Code: NotFound, Msg: "not found", What: "document"}
)
