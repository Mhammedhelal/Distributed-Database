package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"master/internal/cluster"
	"master/internal/query"
	"master/internal/replication"
	"master/internal/wal"
)

type Handler struct {
	exec       *query.Executor
	wal        *wal.WAL
	replicator *replication.Replicator
	registry   *cluster.Registry
	logger     *slog.Logger
}

func New(
	exec *query.Executor,
	w *wal.WAL,
	rep *replication.Replicator,
	reg *cluster.Registry,
	logger *slog.Logger,
) *Handler {
	return &Handler{exec: exec, wal: w, replicator: rep, registry: reg, logger: logger}
}

type queryRequest struct {
	SQL string `json:"sql"`
}

type queryResponse struct {
	Rows         []map[string]any  `json:"rows,omitempty"`
	AffectedRows int64             `json:"affected_rows,omitempty"`
	LastInsertID int64             `json:"last_insert_id,omitempty"`
	Message      string            `json:"message,omitempty"`
	WALSeq       uint64            `json:"wal_seq,omitempty"`
	ReplicaACKs  []replication.ACK `json:"replica_acks,omitempty"`
}

type errorResponse struct {
	Error string `json:"error"`
}

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

	// Bypass custom parser for information_schema and SHOW queries
	sqlLower := strings.ToLower(strings.TrimSpace(req.SQL))
	if strings.Contains(sqlLower, "information_schema") || strings.HasPrefix(sqlLower, "show") {
		rows, err := h.exec.RawSelect(req.SQL)
		if err != nil {
			h.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		h.writeJSON(w, http.StatusOK, queryResponse{Rows: rows})
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

	if entry := result.WALEntry; entry != nil {
		entry.Database = h.exec.DBName()
		if err := h.wal.Append(entry); err != nil {
			h.logger.Error("wal append failed", "err", err)
		}
		resp.WALSeq = entry.Seq
		resp.ReplicaACKs = h.replicator.Replicate(entry)
	}

	h.writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) HealthHandler(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"service": "master",
		"wal_seq": h.wal.LastSeq(),
	})
}

func (h *Handler) ClusterStatusHandler(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, map[string]any{
		"nodes": h.registry.All(),
	})
}

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
