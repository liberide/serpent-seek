package httpd

import (
	"net/http"
	"strings"

	"github.com/liberide/serpent-seek/internal/store"
)

// bulkPayload is the request body for the bulk key/provider endpoints.
type bulkPayload struct {
	Action string   `json:"action"`
	IDs    []string `json:"ids"`
}

// bulkSkip describes one id that could not be processed.
type bulkSkip struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// bulkResult is the shared response of the bulk endpoints. `updated` counts the
// rows actually changed; `skipped` lists ids left untouched with a reason.
type bulkResult struct {
	Updated   int        `json:"updated"`
	Requested int        `json:"requested"`
	Skipped   []bulkSkip `json:"skipped"`
}

const bulkMaxIDs = 500

// parseBulk validates the action and returns the de-duplicated id list.
func parseBulk(w http.ResponseWriter, r *http.Request, allowed ...string) (bulkPayload, []string, bool) {
	var body bulkPayload
	if !readJSON(w, r, &body) {
		return body, nil, false
	}
	action := strings.ToLower(strings.TrimSpace(body.Action))
	valid := false
	for _, a := range allowed {
		if action == a {
			valid = true
			break
		}
	}
	if !valid {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "action must be one of "+strings.Join(allowed, ", "))
		return body, nil, false
	}
	ids := dedupeIDs(body.IDs)
	if len(ids) == 0 {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "ids is required")
		return body, nil, false
	}
	if len(ids) > bulkMaxIDs {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "too many ids (max 500)")
		return body, nil, false
	}
	body.Action = action
	return body, ids, true
}

// dedupeIDs drops empty values while preserving order.
func dedupeIDs(ids []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// handleBulkKeys revokes or deletes several API keys at once.
func (s *Server) handleBulkKeys(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	body, ids, ok := parseBulk(w, r, "revoke", "delete")
	if !ok {
		return
	}
	res := bulkResult{Requested: len(ids), Skipped: []bulkSkip{}}
	for _, id := range ids {
		key, err := s.store.GetAPIKey(ctx, id)
		if err != nil {
			res.Skipped = append(res.Skipped, bulkSkip{ID: id, Reason: "not_found"})
			continue
		}
		if body.Action == "delete" {
			if err := s.store.DeleteAPIKey(ctx, id); err != nil {
				res.Skipped = append(res.Skipped, bulkSkip{ID: id, Reason: "store_error"})
				continue
			}
			res.Updated++
			continue
		}
		if key.RevokedAt == "" {
			key.RevokedAt = store.Now()
			if err := s.store.UpdateAPIKey(ctx, key); err != nil {
				res.Skipped = append(res.Skipped, bulkSkip{ID: id, Reason: "store_error"})
				continue
			}
		}
		res.Updated++
	}
	s.audit(ctx, r, "key.bulk_"+body.Action, "", map[string]string{
		"requested": itoa(res.Requested), "updated": itoa(res.Updated),
	})
	writeJSON(w, http.StatusOK, res)
}

// handleBulkProviders enables, disables or deletes several provider instances
// at once. Deletion skips instances still referenced by a chain.
func (s *Server) handleBulkProviders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	body, ids, ok := parseBulk(w, r, "enable", "disable", "delete")
	if !ok {
		return
	}
	res := bulkResult{Requested: len(ids), Skipped: []bulkSkip{}}
	for _, id := range ids {
		p, err := s.store.GetProvider(ctx, id)
		if err != nil {
			res.Skipped = append(res.Skipped, bulkSkip{ID: id, Reason: "not_found"})
			continue
		}
		if body.Action == "delete" {
			if chains := s.chainsUsingProvider(ctx, id); len(chains) > 0 {
				res.Skipped = append(res.Skipped, bulkSkip{ID: id, Reason: "provider_in_use"})
				continue
			}
			if err := s.store.DeleteProvider(ctx, id); err != nil {
				res.Skipped = append(res.Skipped, bulkSkip{ID: id, Reason: "store_error"})
				continue
			}
			res.Updated++
			continue
		}
		if p.Enabled != (body.Action == "enable") {
			if err := s.store.SetProviderEnabled(ctx, id, body.Action == "enable"); err != nil {
				res.Skipped = append(res.Skipped, bulkSkip{ID: id, Reason: "store_error"})
				continue
			}
		}
		res.Updated++
	}
	s.audit(ctx, r, "provider.bulk_"+body.Action, "", map[string]string{
		"requested": itoa(res.Requested), "updated": itoa(res.Updated),
	})
	writeJSON(w, http.StatusOK, res)
}
