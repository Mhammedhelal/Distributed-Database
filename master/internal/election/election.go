// Package election implements the Bully algorithm for leader election.
//
// When the current master fails, the node with the numerically highest
// numeric ID among alive nodes declares itself the new master.
//
// Message flow:
//  1. Any node that detects master failure starts an election by
//     broadcasting ELECTION messages to all higher-ID nodes.
//  2. Any node that receives ELECTION and has a higher ID responds OK
//     and starts its own election.
//  3. The node that receives no OK responses (highest alive ID) broadcasts
//     COORDINATOR, announcing itself as the new master.
package election

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"master/internal/cluster"
)

// MsgType classifies an election protocol message.
type MsgType string

const (
	MsgElection    MsgType = "ELECTION"
	MsgOK          MsgType = "OK"
	MsgCoordinator MsgType = "COORDINATOR"
)

// Message is exchanged between nodes during an election.
type Message struct {
	Type   MsgType `json:"type"`
	FromID string  `json:"from_id"`
}

// Manager runs leader election for this node.
type Manager struct {
	nodeID   string // e.g. "3"
	nodeNum  int    // numeric form of nodeID
	selfAddr string
	registry *cluster.Registry
	client   *http.Client
	logger   *slog.Logger

	isMaster atomic.Bool
	mu       sync.Mutex
	inElect  bool
}

// New creates a Manager. nodeID must be a numeric string ("1", "2", "3"…).
func New(nodeID, selfAddr string, registry *cluster.Registry, logger *slog.Logger) (*Manager, error) {
	num, err := strconv.Atoi(nodeID)
	if err != nil {
		return nil, fmt.Errorf("nodeID must be numeric, got %q", nodeID)
	}
	return &Manager{
		nodeID:   nodeID,
		nodeNum:  num,
		selfAddr: selfAddr,
		registry: registry,
		client:   &http.Client{Timeout: 2 * time.Second},
		logger:   logger,
	}, nil
}

// SetMaster marks whether this node currently believes itself to be master.
func (m *Manager) SetMaster(v bool) { m.isMaster.Store(v) }

// IsMaster returns true if this node is currently the master.
func (m *Manager) IsMaster() bool { return m.isMaster.Load() }

// StartElection initiates a Bully election.
// Returns true if this node wins (becomes master).
func (m *Manager) StartElection() bool {
	m.mu.Lock()
	if m.inElect {
		m.mu.Unlock()
		return m.isMaster.Load()
	}
	m.inElect = true
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.inElect = false
		m.mu.Unlock()
	}()

	m.logger.Info("election started", "node", m.nodeID)

	// Send ELECTION to all nodes with higher numeric ID.
	higherNodes := m.higherNodes()
	if len(higherNodes) == 0 {
		// No higher nodes — we win immediately.
		m.win()
		return true
	}

	gotOK := false
	var wg sync.WaitGroup
	var okMu sync.Mutex

	for _, n := range higherNodes {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()
			if m.sendMsg(addr, MsgElection) {
				okMu.Lock()
				gotOK = true
				okMu.Unlock()
			}
		}(n.Address)
	}
	wg.Wait()

	if gotOK {
		// A higher node is alive and will take over — wait briefly.
		m.logger.Info("higher node responded OK, stepping back", "node", m.nodeID)
		m.isMaster.Store(false)
		return false
	}

	// No higher node responded — we win.
	m.win()
	return true
}

func (m *Manager) win() {
	m.isMaster.Store(true)
	m.logger.Info("won election, broadcasting COORDINATOR", "node", m.nodeID)
	for _, n := range m.registry.AliveWorkers() {
		go m.sendMsg(n.Address, MsgCoordinator)
	}
}

// ElectionHandler handles POST /internal/election from peer nodes.
func (m *Manager) ElectionHandler(w http.ResponseWriter, r *http.Request) {
	var msg Message
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	switch msg.Type {
	case MsgElection:
		// Respond OK (we're alive) and start our own election.
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(Message{Type: MsgOK, FromID: m.nodeID})
		go m.StartElection()

	case MsgCoordinator:
		m.isMaster.Store(false)
		m.logger.Info("new master elected", "coordinator", msg.FromID)
		w.WriteHeader(http.StatusOK)

	default:
		http.Error(w, "unknown message type", http.StatusBadRequest)
	}
}

// sendMsg sends a message to addr/internal/election.
// Returns true if the remote node responded with OK.
func (m *Manager) sendMsg(addr string, t MsgType) bool {
	body, _ := json.Marshal(Message{Type: t, FromID: m.nodeID})
	resp, err := m.client.Post(addr+"/internal/election", "application/json",
		byteReader(body))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if t == MsgElection {
		var reply Message
		json.NewDecoder(resp.Body).Decode(&reply)
		return reply.Type == MsgOK
	}
	return resp.StatusCode == http.StatusOK
}

func (m *Manager) higherNodes() []*cluster.Node {
	all := m.registry.AliveWorkers()
	var higher []*cluster.Node
	for _, n := range all {
		num, err := strconv.Atoi(n.ID)
		if err != nil {
			continue
		}
		if num > m.nodeNum {
			higher = append(higher, n)
		}
	}
	return higher
}

// byteReader wraps []byte as io.Reader.
type byteReader []byte

func (b byteReader) Read(p []byte) (int, error) {
	if len(b) == 0 {
		return 0, fmt.Errorf("EOF")
	}
	n := copy(p, b)
	return n, nil
}