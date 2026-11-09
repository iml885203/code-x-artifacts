package web

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/iml885203/code-x-artifacts/internal/service"
)

const slideNumberProblem = "the slide number in the path is a whole number, from 1"

func (s *Server) listSlides(w http.ResponseWriter, r *http.Request) {
	list, err := s.svc.Slides(r.Context(), currentViewer(r), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) getSlide(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil {
		jsonError(w, http.StatusBadRequest, slideNumberProblem)
		return
	}
	read, err := s.svc.Slide(r.Context(), currentViewer(r), r.PathValue("id"), n)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, read)
}

// slideBody is the body of a slide edit; each route reads the fields it needs.
type slideBody struct {
	BaseVersion int             `json:"baseVersion"`
	Slide       json.RawMessage `json:"slide"`
	After       *int            `json:"after"`
	To          *int            `json:"to"`
}

// editSlide checks that the caller may edit the deck before it reads the body,
// as a publish does, then runs the edit that build makes of the request.
func (s *Server) editSlide(build func(r *http.Request, in slideBody) (service.SlideEditInput, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.editArtifact(w, r); !ok {
			return
		}
		var in slideBody
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
		if err != nil || (len(body) > 0 && json.Unmarshal(body, &in) != nil) {
			jsonError(w, http.StatusBadRequest, "body must be JSON {slide?, after?, to?, baseVersion?}; under 10 MB")
			return
		}
		edit, problem := build(r, in)
		if problem != "" {
			jsonError(w, http.StatusBadRequest, problem)
			return
		}
		out, err := s.svc.EditSlide(r.Context(), currentViewer(r), r.PathValue("id"), edit)
		if err != nil {
			s.fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func numbered(r *http.Request, edit service.SlideEditInput) (service.SlideEditInput, string) {
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil {
		return edit, slideNumberProblem
	}
	edit.Number = n
	return edit, ""
}

func replaceSlide(r *http.Request, in slideBody) (service.SlideEditInput, string) {
	return numbered(r, service.SlideEditInput{Op: service.SlideReplace, Slide: in.Slide, BaseVersion: in.BaseVersion})
}

func insertSlide(_ *http.Request, in slideBody) (service.SlideEditInput, string) {
	if in.After == nil {
		return service.SlideEditInput{}, `"after" is required: the number of the slide the new one goes after, 0 to put it first`
	}
	return service.SlideEditInput{Op: service.SlideInsert, After: *in.After, Slide: in.Slide, BaseVersion: in.BaseVersion}, ""
}

func moveSlide(r *http.Request, in slideBody) (service.SlideEditInput, string) {
	if in.To == nil {
		return service.SlideEditInput{}, `"to" is required: the number the slide has after the move`
	}
	return numbered(r, service.SlideEditInput{Op: service.SlideMove, To: *in.To, BaseVersion: in.BaseVersion})
}

// deleteSlide takes baseVersion from the query, since a DELETE has no body.
func deleteSlide(r *http.Request, _ slideBody) (service.SlideEditInput, string) {
	edit := service.SlideEditInput{Op: service.SlideDelete}
	if q := r.URL.Query().Get("baseVersion"); q != "" {
		base, err := strconv.Atoi(q)
		if err != nil {
			return edit, "baseVersion must be a version number, from 1"
		}
		edit.BaseVersion = base
	}
	return numbered(r, edit)
}

// routesSlides registers reading and changing one slide of a deck.
func (s *Server) routesSlides(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/artifacts/{id}/slides", s.openAPI(s.listSlides))
	mux.HandleFunc("GET /api/artifacts/{id}/slides/{n}", s.openAPI(s.getSlide))
	mux.HandleFunc("PUT /api/artifacts/{id}/slides/{n}", s.api(s.editSlide(replaceSlide)))
	mux.HandleFunc("POST /api/artifacts/{id}/slides", s.api(s.editSlide(insertSlide)))
	mux.HandleFunc("POST /api/artifacts/{id}/slides/{n}/move", s.api(s.editSlide(moveSlide)))
	mux.HandleFunc("DELETE /api/artifacts/{id}/slides/{n}", s.api(s.editSlide(deleteSlide)))
}
