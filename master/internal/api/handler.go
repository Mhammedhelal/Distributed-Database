// Package api exposes the master node's HTTP API.
package api

import (
	"encoding/json"
	"log/slog"
	"fmt"
	"net/http"

	"master/internal/cluster"
	"master/internal/query"
	"master/internal/replication"
	"master/internal/wal"
)

// Handler wires together all dependencies for the HTTP API.
type Handler struct {
	exec        *query.Executor
	wal         *wal.WAL
	replicator  *replication.Replicator
	registry    *cluster.Registry
	logger      *slog.Logger
}

// New creates a Handler.
func New(
	exec *query.Executor,
	w *wal.WAL,
	rep *replication.Replicator,
	reg *cluster.Registry,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		exec:       exec,
		wal:        w,
		replicator: rep,
		registry:   reg,
		logger:     logger,
	}
}

// ── Request / response shapes ─────────────────────────────────────────────────

type queryRequest struct {
	SQL string `json:"sql"`
}

type queryResponse struct {
	Rows         []map[string]any `json:"rows,omitempty"`
	AffectedRows int64            `json:"affected_rows,omitempty"`
	LastInsertID int64            `json:"last_insert_id,omitempty"`
	Message      string           `json:"message,omitempty"`
	WALSeq       uint64           `json:"wal_seq,omitempty"`
	ReplicaACKs  []replication.ACK `json:"replica_acks,omitempty"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// ── Endpoints ─────────────────────────────────────────────────────────────────

// QueryHandler handles POST /query
// All SQL statements are accepted here; DROP DATABASE is rejected if not master.
func (h *Handler) QueryHandler(w http.ResponseWriter, r *http.Request) {
	var req queryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.SQL == "" {
		h.writeError(w, http.StatusBadRequest, "sql field is required")
		return
	}

	stmt, err := query.Parse(req.SQL)
	if err != nil {
		h.writeError(w, http.StatusUnprocessableEntity, "parse error: "+err.Error())
		return
	}

	result, err := h.exec.Execute(stmt)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := queryResponse{
		Rows:         result.Rows,
		AffectedRows: result.AffectedRows,
		LastInsertID: result.LastInsertID,
		Message:      result.Message,
	}

	// For write operations: append to WAL then replicate.
	if entry := result.WALEntry; entry != nil {
		entry.Database = h.exec.DBName()
		if err := h.wal.Append(entry); err != nil {
			h.logger.Error("wal append failed", "err", err)
			// Don't fail the write — log and continue.
		}
		resp.WALSeq = entry.Seq
		resp.ReplicaACKs = h.replicator.Replicate(entry)
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// HealthHandler handles GET /health
func (h *Handler) HealthHandler(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"service": "master",
		"wal_seq": h.wal.LastSeq(),
	})
}

// ClusterStatusHandler handles GET /cluster/status
func (h *Handler) ClusterStatusHandler(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, map[string]any{
		"nodes": h.registry.All(),
	})
}

// WALSinceHandler handles GET /internal/wal?after=<seq>
// Used by rejoining workers to catch up.
func (h *Handler) WALSinceHandler(w http.ResponseWriter, r *http.Request) {
	var afterSeq uint64
	if s := r.URL.Query().Get("after"); s != "" {
		var n uint64
		if _, err := scanUint(s, &n); err == nil {
			afterSeq = n
		}
	}
	entries, err := h.wal.Since(afterSeq)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (h *Handler) writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		h.logger.Error("encode response", "err", err)
	}
}

func (h *Handler) writeError(w http.ResponseWriter, code int, msg string) {
	h.writeJSON(w, code, errorResponse{Error: msg})
}

func scanUint(s string, out *uint64) (int, error) {
	var n uint64
	_, err := fmt.Sscanf(s, "%d", &n)
	if err != nil {
		return 0, err
	}
	*out = n
	return 1, nil
}