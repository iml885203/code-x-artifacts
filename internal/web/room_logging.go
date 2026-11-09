package web

import (
	"context"
	"log/slog"

	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/operationlog"
	"github.com/iml885203/code-x-artifacts/internal/service"
)

// Snapshots can be saved by a timer or a request flushing a room. Attach the
// sink at the Versions boundary so neither path depends on a request living on.
type loggedRoomVersions struct {
	svc *service.Service
	log *slog.Logger
}

func (v loggedRoomVersions) SaveFromRoom(ctx context.Context, id string, base int, bundle kind.Bundle, authors []string) error {
	return v.svc.SaveFromRoom(operationlog.Context(ctx, v.log), id, base, bundle, authors)
}
