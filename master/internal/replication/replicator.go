// Package replication fans WAL entries out to all alive worker nodes.
package replication

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"master/internal/cluster"
	"master/internal/wal"
)

const (
	maxRetries    = 3
	retryBaseWait = 200 * time.Millisecond
	applyTimeout  = 5 * time.Second
)

// ACK is the response a worker sends after applying a WAL entry.
type ACK struct {
	NodeID string `json:"node_id"`
	Seq    uint64 `json:"seq"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
}

// Replicator fans a WAL entry out to all alive workers.
type Replicator struct {
	registry *cluster.Registry
	signer   Signer
	client   *http.Client
	logger   *slog.Logger
}

// Signer is satisfied by auth.Signer — defined as an interface to avoid
// an import cycle between replication and auth.
type Signer interface {
	Generate() string
}

// New creates a Replicator.
func New(registry *cluster.Registry, signer Signer, logger *slog.Logger) *Replicator {
	return &Replicator{
		registry: registry,
		signer:   signer,
		client:   &http.Client{Timeout: applyTimeout},
		logger:   logger,
	}
}

// Replicate sends entry to every alive worker concurrently.
// It returns one ACK per worker. Unreachable workers are retried up to
// maxRetries times with exponential back-off; failures are logged but do
// not block the write path.
func (r *Replicator) Replicate(entry *wal.Entry) []ACK {
	workers := r.registry.AliveWorkers()
	if len(workers) == 0 {
		r.logger.Warn("no alive workers to replicate to", "seq", entry.Seq)
		return nil
	}

	var mu sync.Mutex
	var acks []ACK
	var wg sync.WaitGroup

	for _, w := range workers {
		wg.Add(1)
		go func(node *cluster.Node) {
			defer wg.Done()
			ack := r.sendWithRetry(node, entry)
			if ack.OK {
				r.registry.MarkACK(node.ID, entry.Seq)
			}
			mu.Lock()
			acks = append(acks, ack)
			mu.Unlock()
		}(w)
	}
	wg.Wait()
	return acks
}

func (r *Replicator) sendWithRetry(node *cluster.Node, entry *wal.Entry) ACK {
	body, err := json.Marshal(entry)
	if err != nil {
		return ACK{NodeID: node.ID, Error: fmt.Sprintf("marshal: %v", err)}
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			wait := retryBaseWait * time.Duration(1<<attempt)
			time.Sleep(wait)
		}
		ack, err := r.send(node.Address, body, entry.Seq)
		if err == nil && ack.OK {
			return ack
		}
		r.logger.Warn("replication attempt failed",
			"node", node.ID, "attempt", attempt+1, "err", err)
	}
	return ACK{NodeID: node.ID, Seq: entry.Seq,
		Error: fmt.Sprintf("failed after %d attempts", maxRetries)}
}

func (r *Replicator) send(addr string, body []byte, seq uint64) (ACK, error) {
	ctx, cancel := context.WithTimeout(context.Background(), applyTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		addr+"/replication/apply", bytes.NewReader(body))
	if err != nil {
		return ACK{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Master-Token", r.signer.Generate())

	resp, err := r.client.Do(req)
	if err != nil {
		return ACK{}, err
	}
	defer resp.Body.Close()

	var ack ACK
	if err := json.NewDecoder(resp.Body).Decode(&ack); err != nil {
		return ACK{}, fmt.Errorf("decode ack: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return ACK{}, fmt.Errorf("worker returned HTTP %d", resp.StatusCode)
	}
	return ack, nil
}

// CatchUp sends all WAL entries after afterSeq to a specific node.
// Called when a node rejoins after downtime.
func (r *Replicator) CatchUp(nodeID string, afterSeq uint64, entries []wal.Entry) {
	node, ok := r.registry.Get(nodeID)
	if !ok {
		r.logger.Error("catch-up: node not found", "id", nodeID)
		return
	}
	r.logger.Info("starting catch-up", "node", nodeID, "from_seq", afterSeq, "entries", len(entries))
	for i := range entries {
		ack := r.sendWithRetry(&node, &entries[i])
		if !ack.OK {
			r.logger.Error("catch-up failed at entry", "seq", entries[i].Seq, "err", ack.Error)
			return
		}
	}
	r.logger.Info("catch-up complete", "node", nodeID)
}