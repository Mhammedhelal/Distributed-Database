package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"sync"
	"time"
)

// ==========================================
// 1. Main Server (Permanent Storage)
// ==========================================
type MainServer struct{}

func (m *MainServer) SaveData(data string, reply *bool) error {
	fmt.Printf("[Main Server] 💾 Data received and saved permanently: '%s'\n", data)
	*reply = true
	return nil
}

func runMainServer() {
	mainSrv := new(MainServer)
	rpc.Register(mainSrv)
	listener, err := net.Listen("tcp", ":5001")
	if err != nil {
		log.Fatal("Main Server error:", err)
	}
	fmt.Println("🟢 Main Server running on port 5001...")
	for {
		conn, _ := listener.Accept()
		go rpc.ServeConn(conn)
	}
}

// ==========================================
// 2. Cache Server (Temporary Storage)
// ==========================================
type CacheServer struct {
	mu    sync.Mutex
	store map[string]string
}

func (c *CacheServer) InsertData(data string, reply *string) error {
	// 1. Save data to Cache
	c.mu.Lock()
	key := fmt.Sprintf("key_%d", time.Now().UnixNano())
	c.store[key] = data
	fmt.Printf("\n[Cache Server] 🟡 Data temporarily stored with key: %s\n", key)
	c.mu.Unlock()

	// 2. Forward data to Main Server
	client, err := rpc.Dial("tcp", "localhost:5001")
	if err != nil {
		return fmt.Errorf("could not connect to Main Server: %v", err)
	}
	defer client.Close()

	var success bool
	fmt.Println("[Cache Server] ⏳ Forwarding data to Main Server...")
	err = client.Call("MainServer.SaveData", data, &success)

	// 3. Delete from Cache if successfully saved in Main
	if err == nil && success {
		c.mu.Lock()
		delete(c.store, key)
		fmt.Printf("[Cache Server] 🔴 Data successfully forwarded and deleted from Cache.\n")
		c.mu.Unlock()
		*reply = "Success: Data saved in Main and cleared from Cache."
	} else {
		*reply = "Failed to forward data."
	}

	return nil
}

func runCacheServer() {
	cacheSrv := &CacheServer{store: make(map[string]string)}
	rpc.Register(cacheSrv)
	listener, err := net.Listen("tcp", ":5000")
	if err != nil {
		log.Fatal("Cache Server error:", err)
	}
	fmt.Println("🟡 Cache Server running on port 5000...")
	for {
		conn, _ := listener.Accept()
		go rpc.ServeConn(conn)
	}
}

// ==========================================
// 3. Client (Data Sender)
// ==========================================
func runClient(data string) {
	fmt.Printf("[Client] 📤 Sending data to Cache Server: '%s'\n", data)
	client, err := rpc.Dial("tcp", "localhost:5000")
	if err != nil {
		log.Fatal("Client failed to connect to Cache:", err)
	}
	defer client.Close()

	var reply string
	err = client.Call("CacheServer.InsertData", data, &reply)
	if err != nil {
		log.Fatal("Client error:", err)
	}
	fmt.Printf("[Client] ✅ Response: %s\n", reply)
}

// ==========================================
// 4. Main Entry Point
// ==========================================
func main() {
	mode := flag.String("mode", "main", "Run mode: 'main', 'cache', or 'client'")
	data := flag.String("data", "Hello Distributed Systems!", "Data to send (for client mode)")
	flag.Parse()

	switch *mode {
	case "main":
		runMainServer()
	case "cache":
		runCacheServer()
	case "client":
		runClient(*data)
	default:
		fmt.Println("Invalid mode. Use 'main', 'cache', or 'client'")
	}
}