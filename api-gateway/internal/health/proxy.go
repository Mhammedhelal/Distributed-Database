// Package health provides a health-proxy that fans out to all cluster nodes
// and returns a unified JSON status document at GET /cluster/status.
package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// NodeStatus holds the last known health of a single node.
type NodeStatus struct {
	ID      string `json:"id"`
	Address string `json:"address"`
	Role    string `json:"role"`   // "master" | "worker"
	Status  string `json:"status"` // "alive" | "down"
	Latency string `json:"latency_ms,omitempty"`
	Error   string `json:"error,omitempty"`
}

// ClusterStatus is the aggregate response returned at /cluster/status.
type ClusterStatus struct {
	CheckedAt time.Time    `json:"checked_at"`
	Nodes     []NodeStatus `json:"nodes"`
}

// Proxy aggregates /health responses from all known nodes.
type Proxy struct {
	client  *http.Client
	master  nodeRef
	workers []nodeRef
}

type nodeRef struct {
	id      string
	address string
	role    string
}

// New creates a Proxy for the given master and worker addresses.
func New(masterID, masterAddr string, workers []struct{ ID, Address string }) *Proxy {
	refs := make([]nodeRef, len(workers))
	for i, w := range workers {
		refs[i] = nodeRef{id: w.ID, address: w.Address, role: "worker"}
	}
	return &Proxy{
		client:  &http.Client{Timeout: 3 * time.Second},
		master:  nodeRef{id: masterID, address: masterAddr, role: "master"},
		workers: refs,
	}
}

// Handler returns an http.HandlerFunc for GET /cluster/status.
func (p *Proxy) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		all := append([]nodeRef{p.master}, p.workers...)

		results := make([]NodeStatus, len(all))
		var wg sync.WaitGroup
		for i, n := range all {
			wg.Add(1)
			go func(idx int, node nodeRef) {
				defer wg.Done()
				results[idx] = p.probe(r.Context(), node)
			}(i, n)
		}
		wg.Wait()

		cs := ClusterStatus{
			CheckedAt: time.Now().UTC(),
			Nodes:     results,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cs) //nolint:errcheck
	}
}

// probe sends a GET /health to the node and returns its status.
func (p *Proxy) probe(ctx context.Context, n nodeRef) NodeStatus {
	ns := NodeStatus{ID: n.id, Address: n.address, Role: n.role}

	url := n.address + "/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		ns.Status = "down"
		ns.Error = fmt.Sprintf("build request: %v", err)
		return ns
	}

	start := time.Now()
	resp, err := p.client.Do(req)
	latency := time.Since(start)

	if err != nil {
		ns.Status = "down"
		ns.Error = err.Error()
		return ns
	}
	defer resp.Body.Close()

	ns.Latency = fmt.Sprintf("%.1f", float64(latency.Milliseconds()))
	if resp.StatusCode == http.StatusOK {
		ns.Status = "alive"
	} else {
		ns.Status = "down"
		ns.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return ns
}