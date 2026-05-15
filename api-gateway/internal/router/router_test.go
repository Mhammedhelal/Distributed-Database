package router_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"api-gateway/internal/auth"
	"api-gateway/internal/router"
)

// newTestBackend creates a test HTTP server that records received headers
// and responds with statusCode.
func newTestBackend(t *testing.T, statusCode int) (*httptest.Server, *http.Header) {
	t.Helper()
	var captured http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Header.Clone()
		w.WriteHeader(statusCode)
	}))
	t.Cleanup(srv.Close)
	return srv, &captured
}

func newRouter(masterURL string, workerURLs []string) *router.Router {
	signer := auth.NewSigner("test-secret", 30*time.Second)
	return router.New(router.Config{
		MasterURL:  masterURL,
		WorkerURLs: workerURLs,
		Signer:     signer,
		Logger:     slog.Default(),
	})
}

func TestPostQueryGoesToMaster(t *testing.T) {
	master, _ := newTestBackend(t, http.StatusOK)
	worker, _ := newTestBackend(t, http.StatusOK)

	rt := newRouter(master.URL, []string{worker.URL})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/query", strings.NewReader(`{"sql":"INSERT INTO t VALUES(1)"}`))
	req.Header.Set("Content-Type", "application/json")

	rt.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestGetQueryGoesToWorker(t *testing.T) {
	// Master returns 500 to catch accidental routing to it.
	master, _ := newTestBackend(t, http.StatusInternalServerError)
	worker, _ := newTestBackend(t, http.StatusOK)

	rt := newRouter(master.URL, []string{worker.URL})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/query?sql=SELECT+1", nil)
	rt.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /query should route to worker, got %d", rec.Code)
	}
}

func TestReplicationBlockedFromOutside(t *testing.T) {
	master, _ := newTestBackend(t, http.StatusOK)
	rt := newRouter(master.URL, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/replication/apply", strings.NewReader("{}"))
	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestAdminGoesToMaster(t *testing.T) {
	master, _ := newTestBackend(t, http.StatusOK)
	worker, _ := newTestBackend(t, http.StatusInternalServerError)

	rt := newRouter(master.URL, []string{worker.URL})

	for _, path := range []string{"/admin/create-db", "/admin/drop-db"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
		rt.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("POST %s should route to master, got %d", path, rec.Code)
		}
	}
}

func TestStripExternalMasterToken(t *testing.T) {
	// Capture the headers the worker receives.
	var workerHeaders http.Header
	workerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		workerHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer workerSrv.Close()

	master, _ := newTestBackend(t, http.StatusInternalServerError) // shouldn't be called
	rt := newRouter(master.URL, []string{workerSrv.URL})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/query?sql=SELECT+1", nil)
	// Simulate a client trying to spoof the master token.
	req.Header.Set("X-Master-Token", "forged-token")

	// Run through the strip middleware, then the router.
	handler := auth.StripExternalMasterToken(rt)
	handler.ServeHTTP(rec, req)

	if tok := workerHeaders.Get("X-Master-Token"); tok != "" {
		t.Fatalf("X-Master-Token should be stripped but worker received: %q", tok)
	}
}

func TestRoundRobinWorkerSelection(t *testing.T) {
	hits := make([]int, 2)
	workers := make([]*httptest.Server, 2)
	for i := range workers {
		idx := i
		workers[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits[idx]++
			w.WriteHeader(http.StatusOK)
		}))
		defer workers[i].Close()
	}

	master, _ := newTestBackend(t, http.StatusInternalServerError)
	rt := newRouter(master.URL, []string{workers[0].URL, workers[1].URL})

	for i := 0; i < 4; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/query?sql=SELECT+1", nil)
		rt.ServeHTTP(rec, req)
	}

	if hits[0] != 2 || hits[1] != 2 {
		t.Fatalf("expected 2 hits each, got worker0=%d worker1=%d", hits[0], hits[1])
	}
}

func TestConsistencyHeaderOnSlaveRead(t *testing.T) {
	worker, _ := newTestBackend(t, http.StatusOK)
	master, _ := newTestBackend(t, http.StatusInternalServerError)
	rt := newRouter(master.URL, []string{worker.URL})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/query?sql=SELECT+1", nil)
	rt.ServeHTTP(rec, req)

	if v := rec.Header().Get("X-Consistency-Level"); v != "eventual" {
		t.Fatalf("expected X-Consistency-Level: eventual, got %q", v)
	}
}

func TestNoWorkersReturns503(t *testing.T) {
	master, _ := newTestBackend(t, http.StatusOK)
	rt := newRouter(master.URL, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/query?sql=SELECT+1", nil)
	rt.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestUnknownRouteReturns404(t *testing.T) {
	master, _ := newTestBackend(t, http.StatusOK)
	rt := newRouter(master.URL, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	rt.ServeHTTP(rec, req)

	body, _ := io.ReadAll(rec.Body)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d — body: %s", rec.Code, body)
	}
}