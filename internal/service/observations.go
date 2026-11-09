package service

import "context"

// Observation reports an operation's audit fact or a secondary failure that
// must not replace its result (for example, a failed cleanup after a commit).
// The request or background caller decides where to write it; service owns no logger.
type Observation struct {
	Severity Severity
	Message  string
	Fields   []any
}

// Severity distinguishes successful operations from recoverable and failed cleanup.
type Severity uint8

// Operation observation severities.
const (
	Information Severity = iota
	Warning
	Failure
)

type observationKey struct{}

// WithObservations attaches synchronous reporting for this call and nested
// operations. No context or observer is retained by Service. The caller's
// observer must not panic or alter operation state.
func WithObservations(ctx context.Context, observe func(Observation)) context.Context {
	return context.WithValue(ctx, observationKey{}, observe)
}

func report(ctx context.Context, severity Severity, message string, fields ...any) {
	if observe, ok := ctx.Value(observationKey{}).(func(Observation)); ok && observe != nil {
		observe(Observation{Severity: severity, Message: message, Fields: fields})
	}
}
