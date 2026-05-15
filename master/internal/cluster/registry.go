// Package cluster manages the live set of worker nodes and their health status.
package cluster

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Role identifies a node's function in the cluster.
type Role string

const (
	RoleMaster Role = "master"
	RoleWorker Role = "worker"
)

// Status is the last known liveness of a node.
type Status string

const (
	StatusAlive Status = "alive"
	StatusDown  Status = "down"
)

// Node holds runtime information about a cluster member.
type Node struct {
	ID          string    `json:"id"`
	Address     string    `json:"address"`
	Role        Role      `json:"role"`
	Status      Status    `json:"status"`
	LastSeen    time.Time `json:"last_seen"`
	LastSeqACK  uint64    `json:"last_seq_ack"` // highest WAL seq the node has ACKed
}

// Registry keeps the authoritative list of all nodes.
type Registry struct {
	mu     sync.RWMutex
	nodes  map[string]*Node
	logger *slog.Logger

	missThreshold int           // consecutive missed heartbeats before marking down
	interval      time.Duration // heartbeat probe interval
}

// NewRegistry creates a Registry and starts the heartbeat probe loop.
func NewRegistry(interval time.Duration, missThreshold int, logger *slog.Logger) *Registry {
	r := &Registry{
		nodes:         make(map[string]*Node),
		logger:        logger,
		missThreshold: missThreshold,
		interval:      interval,
	}
	go r.probeLoop()
	return r
}

// Register adds or updates a node entry.
func (r *Registry) Register(n Node) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n.LastSeen = time.Now()
	r.nodes[n.ID] = &n
}

// MarkACK updates the highest WAL sequence a worker has acknowledged.
func (r *Registry) MarkACK(id string, seq uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n, ok := r.nodes[id]; ok {
		if seq > n.LastSeqACK {
			n.LastSeqACK = seq
		}
	}
}

// AliveWorkers returns all nodes currently marked alive with role Worker.
func (r *Registry) AliveWorkers() []*Node {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []*Node
	for _, n := range r.nodes {
		if n.Role == RoleWorker && n.Status == StatusAlive {
			out = append(out, n)
		}
	}
	return out
}

// All returns a snapshot of every registered node.
func (r *Registry) All() []Node {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Node, 0, len(r.nodes))
	for _, n := range r.nodes {
		out = append(out, *n)
	}
	return out
}

// Get returns a single node by ID, false if not found.
func (r *Registry) Get(id string) (Node, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n, ok := r.nodes[id]
	if !ok {
		return Node{}, false
	}
	return *n, true
}

// probeLoop periodically sends GET /health to every registered node and
// marks nodes down after missThreshold consecutive failures.
func (r *Registry) probeLoop() {
	client := &http.Client{Timeout: 3 * time.Second}
	missed := make(map[string]int)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for range ticker.C {
		r.mu.RLock()
		ids := make([]string, 0, len(r.nodes))
		addrs := make(map[string]string)
		for id, n := range r.nodes {
			ids = append(ids, id)
			addrs[id] = n.Address
		}
		r.mu.RUnlock()

		for _, id := range ids {
			resp, err := client.Get(addrs[id] + "/health")
			if err != nil || resp.StatusCode != http.StatusOK {
				missed[id]++
				if missed[id] >= r.missThreshold {
					r.setStatus(id, StatusDown)
					r.logger.Warn("node marked down", "id", id, "misses", missed[id])
				}
				continue
			}
			resp.Body.Close()
			missed[id] = 0
			r.mu.Lock()
			if n, ok := r.nodes[id]; ok {
				n.Status = StatusAlive
				n.LastSeen = time.Now()
			}
			r.mu.Unlock()
		}
	}
}

func (r *Registry) setStatus(id string, s Status) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n, ok := r.nodes[id]; ok {
		n.Status = s
	}
}

// HeartbeatHandler handles POST /internal/heartbeat from workers.
// Workers call this to register themselves and confirm they are alive.
func (r *Registry) HeartbeatHandler(w http.ResponseWriter, req *http.Request) {
	var n Node
	if err := json.NewDecoder(req.Body).Decode(&n); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	n.Status = StatusAlive
	r.Register(n)
	r.logger.Info("heartbeat received", "id", n.ID, "addr", n.Address)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"ok":true,"master_seq":%d}`, 0)
}