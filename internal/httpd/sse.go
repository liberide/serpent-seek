package httpd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/liberide/serpent-seek/internal/engine"
	"github.com/liberide/serpent-seek/internal/sse"
)

// handleRequestEvents streams the live step trace for one request. For a
// finished request it replays the persisted steps and closes with
// request_done, so late subscribers always receive the full trace.
func (s *Server) handleRequestEvents(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, r, http.StatusInternalServerError, "sse_unsupported", "streaming unsupported")
		return
	}
	lastID := lastEventID(r)

	ch, past, unsub := s.hub.Subscribe(id, lastID)
	defer unsub()

	req, err := s.store.GetRequest(r.Context(), id)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "request not found")
		return
	}

	setSSEHeaders(w)
	fmt.Fprint(w, "retry: 2000\n\n")
	flusher.Flush()

	// A finished request with no buffered events (e.g. after a restart) is
	// replayed from the database.
	if req.Status != "running" && len(past) == 0 {
		steps, _ := s.store.ListSteps(r.Context(), id)
		for i, step := range steps {
			payload := engine.StepFinished{
				Idx: step.Idx, Node: step.NodeKey, Provider: step.Provider, Engine: step.Engine,
				Attempt: step.AttemptNo - 1, HTTP: step.HTTPStatus, Kind: step.Kind,
				Results: step.ResultsCount, TookMS: step.TookMS, Permanent: step.Permanent, Error: step.Error,
			}
			writeEvent(w, sse.Event{ID: int64(i + 1), Type: engine.EventStepFinished, Data: mustJSON(payload)})
		}
		done := engine.RequestDone{
			ID: req.ID, RID: req.RID, Status: req.Status, Used: req.UsedProvider,
			Steps: req.StepsCount, TotalMS: req.TotalMS, Results: req.ResultsCount, Error: req.Error,
		}
		writeEvent(w, sse.Event{ID: int64(len(steps)) + 1, Type: engine.EventRequestDone, Data: mustJSON(done)})
		flusher.Flush()
		return
	}

	for _, ev := range past {
		writeEvent(w, ev)
		if ev.Type == engine.EventRequestDone {
			flusher.Flush()
			return
		}
	}
	flusher.Flush()

	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev, open := <-ch:
			if !open {
				return
			}
			writeEvent(w, ev)
			flusher.Flush()
			if ev.Type == engine.EventRequestDone {
				return
			}
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

// handleGlobalEvents streams request_created / request_done for the dashboard
// and history list.
func (s *Server) handleGlobalEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, r, http.StatusInternalServerError, "sse_unsupported", "streaming unsupported")
		return
	}
	ch, past, unsub := s.hub.Subscribe("", lastEventID(r))
	defer unsub()
	setSSEHeaders(w)
	fmt.Fprint(w, "retry: 2000\n\n")
	for _, ev := range past {
		writeEvent(w, ev)
	}
	flusher.Flush()
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev, open := <-ch:
			if !open {
				return
			}
			writeEvent(w, ev)
			flusher.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

func setSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
}

func writeEvent(w http.ResponseWriter, ev sse.Event) {
	fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.ID, ev.Type, ev.Data)
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func lastEventID(r *http.Request) int64 {
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	if v := r.URL.Query().Get("last_event_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return 0
}
