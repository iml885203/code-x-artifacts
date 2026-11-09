package web

import (
	"net/http"
	"time"

	"github.com/iml885203/code-x-artifacts/internal/kind"
	"github.com/iml885203/code-x-artifacts/internal/service"
)

// rawLive answers an address with a live grant: a frame of a design as its room
// holds it now, for the account the grant was made for (service.LiveFrame), so
// a frame someone has just added is there before any version holds it. It is
// the only address that serves what the room holds: a page grant reads its
// version alone. It serves frames only, in the same sandbox and with the same
// headers as a version's frame; everything it cannot read is the same 404 as a
// bad grant.
func (s *Server) rawLive(w http.ResponseWriter, r *http.Request, id, path string) {
	account, ok := s.sessions.openLiveGrant(r.PathValue("grant"), id, time.Now())
	if !ok {
		jsonError(w, http.StatusNotFound, "not found")
		return
	}
	a, text, err := s.svc.LiveFrame(r.Context(), service.LiveGrant{ArtifactID: id, Account: account}, path)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.writeFrame(w, r, a, kind.Design{}, path, s.assetGrants(a).Replace(text))
}
