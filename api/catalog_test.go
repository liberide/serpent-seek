package api

import "testing"

func TestOperationsCatalog(t *testing.T) {
	ops := Operations()
	if len(ops) < 50 {
		t.Fatalf("expected a rich operation catalog, got %d", len(ops))
	}
	byID := make(map[string]Operation, len(ops))
	for _, op := range ops {
		if op.ID != op.Method+" "+op.Path {
			t.Errorf("bad id %q for %s %s", op.ID, op.Method, op.Path)
		}
		if op.Method == "" || op.Path == "" {
			t.Errorf("incomplete operation: %+v", op)
		}
		byID[op.ID] = op
	}

	for _, id := range []string{
		"GET /api/chains",
		"POST /api/chains",
		"DELETE /api/providers/{id}",
		"POST /api/developer/tokens",
		"PATCH /api/developer/tokens/{id}",
	} {
		if _, ok := byID[id]; !ok {
			t.Errorf("catalog is missing %q", id)
		}
	}

	if op := byID["GET /api/chains"]; op.Tag != "chains" {
		t.Errorf("GET /api/chains tag = %q, want chains", op.Tag)
	}
	if op := byID["GET /version"]; !op.Public {
		t.Errorf("GET /version should be marked public")
	}
	if op := byID["GET /api/chains"]; op.Public {
		t.Errorf("GET /api/chains must not be public")
	}
	if op := byID["GET /api/chains"]; op.Summary == "" {
		t.Errorf("GET /api/chains summary should be parsed")
	}
}
