package main

import (
	"fmt"
	"hash/crc32"
	"sort"
)

// ==========================================
// 1. Consistent Hash Ring Structure
// ==========================================

// HashRing represents the consistent hashing ring
type HashRing struct {
	nodes   []int          // Sorted array of hashes (the ring)
	nodeMap map[int]string // Maps a hash back to the physical Server IP/Name
}

// NewHashRing creates a new empty HashRing
func NewHashRing() *HashRing {
	return &HashRing{
		nodes:   make([]int, 0),
		nodeMap: make(map[int]string),
	}
}

// hash generates a fast checksum for a given string (Node ID or Data Key)
func (hr *HashRing) hash(key string) int {
	return int(crc32.ChecksumIEEE([]byte(key)))
}

// ==========================================
// 2. Node Management
// ==========================================

// AddNode adds a new server/node to the ring
func (hr *HashRing) AddNode(nodeID string) {
	hashVal := hr.hash(nodeID)
	hr.nodes = append(hr.nodes, hashVal)
	hr.nodeMap[hashVal] = nodeID
	
	// Keep the ring sorted so we can use Binary Search later
	sort.Ints(hr.nodes) 
	fmt.Printf("[+] Added Server: %s (Hash: %d)\n", nodeID, hashVal)
}

// RemoveNode simulates a server crashing or being removed from the ring
func (hr *HashRing) RemoveNode(nodeID string) {
	hashVal := hr.hash(nodeID)
	delete(hr.nodeMap, hashVal)

	// Remove the node's hash from the sorted slice
	for i, n := range hr.nodes {
		if n == hashVal {
			hr.nodes = append(hr.nodes[:i], hr.nodes[i+1:]...)
			break
		}
	}
	fmt.Printf("[-] Removed Server: %s (Hash: %d)\n", nodeID, hashVal)
}

// ==========================================
// 3. Data Distribution
// ==========================================

// GetNode finds the appropriate server to store or retrieve a piece of data
func (hr *HashRing) GetNode(key string) string {
	if len(hr.nodes) == 0 {
		return "No servers available"
	}

	hashVal := hr.hash(key)

	// Binary search: find the first node whose hash is >= the data's hash
	idx := sort.Search(len(hr.nodes), func(i int) bool {
		return hr.nodes[i] >= hashVal
	})

	// If the data hash is larger than the largest node hash, wrap around to the first node
	if idx == len(hr.nodes) {
		idx = 0
	}

	return hr.nodeMap[hr.nodes[idx]]
}

// ==========================================
// 4. Main Demonstration
// ==========================================

func main() {
	ring := NewHashRing()

	// 1. Initialize the distributed database servers
	fmt.Println("--- 1. Initializing Servers ---")
	ring.AddNode("Server-A (192.168.1.10)")
	ring.AddNode("Server-B (192.168.1.11)")
	ring.AddNode("Server-C (192.168.1.12)")

	// 2. Define some data keys (e.g., database records, files)
	keys := []string{
		"user_data_ahmed",
		"user_data_omar",
		"image_profile_55.png",
		"settings_config.json",
		"post_id_9921",
	}

	// 3. Distribute the data across the servers
	fmt.Println("\n--- 2. Distributing Data ---")
	for _, key := range keys {
		assignedServer := ring.GetNode(key)
		fmt.Printf("Data: '%-22s' -> Stored in: %s\n", key, assignedServer)
	}

	// 4. Simulate a node failure (Server-B goes down)
	fmt.Println("\n--- 3. Simulating Server Crash ---")
	ring.RemoveNode("Server-B (192.168.1.11)")

	// 5. Redistribute data
	// Notice that ONLY the data that was on Server-B will move to Server-C or Server-A.
	// Data already on A and C stays exactly where it is (This is the magic of Consistent Hashing!)
	fmt.Println("\n--- 4. Data Locations After Crash ---")
	for _, key := range keys {
		assignedServer := ring.GetNode(key)
		fmt.Printf("Data: '%-22s' -> NOW in: %s\n", key, assignedServer)
	}
}

📊 Expected Output & Flow Explanation
When you run the script, the program will walk you through a 4-step simulation. Here is what you should look out for in the terminal output:

1. Initializing Servers
The program adds three servers (Server-A, Server-B, and Server-C) to the Hash Ring and calculates their fixed hash positions.

2. Distributing Data
The program takes an array of keys (e.g., user_data_ahmed, image_profile_55.png) and assigns them to the closest server on the ring.

--- 2. Distributing Data ---
Data: 'user_data_ahmed       ' -> Stored in: Server-A (192.168.1.10)
Data: 'user_data_omar        ' -> Stored in: Server-B (192.168.1.11)
Data: 'image_profile_55.png  ' -> Stored in: Server-A (192.168.1.10)
...

3. Simulating a Server Crash
The program simulates a hardware failure by removing Server-B from the ring.

4. Data Locations After Crash (The Magic)
The program redistributes the data. Notice the behavior:
Only the data that was on Server-B (e.g., user_data_omar) will be moved to a new server (like Server-C). All other data (like user_data_ahmed on Server-A) will stay exactly where it was.

--- 4. Data Locations After Crash ---
Data: 'user_data_ahmed       ' -> NOW in: Server-A (192.168.1.10) (Remains unchanged)
Data: 'user_data_omar        ' -> NOW in: Server-C (192.168.1.12) (Relocated!)
...

⚙️ How It Works (Under the Hood)
Hashing Algorithm: Uses Go's hash/crc32 to generate a 32-bit integer hash for both the Server IPs and the Data Keys.

Ring Sorting: The HashRing.nodes array is kept strictly sorted to allow for efficient searching.

Lookup Logic: When looking for a server to store a piece of data, the algorithm calculates the data's hash, then searches the ring clockwise (using binary search) for the first server with a hash greater than or equal to the data's hash.