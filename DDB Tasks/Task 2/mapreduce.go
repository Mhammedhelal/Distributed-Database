package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"strings"
	"sync"
)

// ==========================================
// 1. Shared Structures
// ==========================================

// QueryArgs represents the data sent from Master to Worker
type QueryArgs struct {
	Keyword string
}

// QueryReply represents the data sent back from Worker to Master
type QueryReply struct {
	Results []string
}

// ==========================================
// 2. Worker Node (RPC Server)
// ==========================================

type Worker struct {
	LocalData []string
}

// ExecuteQuery is the RPC method that the Master will call remotely
func (w *Worker) ExecuteQuery(args *QueryArgs, reply *QueryReply) error {
	log.Printf("Executing query for keyword: '%s'", args.Keyword)
	
	var matched []string
	// Map Phase: Filter local data based on the query
	for _, line := range w.LocalData {
		if strings.Contains(strings.ToLower(line), strings.ToLower(args.Keyword)) {
			matched = append(matched, line)
		}
	}
	
	reply.Results = matched
	return nil
}

func runWorker(port string, data []string) {
	worker := &Worker{LocalData: data}
	rpc.Register(worker)

	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Worker failed to listen: %v", err)
	}
	
	fmt.Printf("Worker started on port %s with %d records.\n", port, len(data))

	// Keep accepting incoming RPC connections from the Master
	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		go rpc.ServeConn(conn)
	}
}

// ==========================================
// 3. Master Node (RPC Client)
// ==========================================

func runMaster(workerAddresses []string, keyword string) {
	var allResults []string
	var wg sync.WaitGroup
	var mu sync.Mutex // To prevent race conditions when appending results

	fmt.Printf("Master sending query '%s' to workers: %v\n", keyword, workerAddresses)

	// Send the query to all workers concurrently
	for _, addr := range workerAddresses {
		wg.Add(1)
		go func(workerAddress string) {
			defer wg.Done()

			client, err := rpc.Dial("tcp", workerAddress)
			if err != nil {
				log.Printf("Failed to connect to worker at %s: %v", workerAddress, err)
				return
			}
			defer client.Close()

			args := QueryArgs{Keyword: keyword}
			var reply QueryReply

			// Call the remote function
			err = client.Call("Worker.ExecuteQuery", &args, &reply)
			if err != nil {
				log.Printf("RPC error from %s: %v", workerAddress, err)
				return
			}

			// Reduce Phase: Aggregate the results
			mu.Lock()
			allResults = append(allResults, reply.Results...)
			mu.Unlock()
			
			fmt.Printf("Received %d matches from %s\n", len(reply.Results), workerAddress)
		}(addr)
	}

	// Wait for all workers to finish processing
	wg.Wait()

	// Display aggregated results
	fmt.Println("\n--- Final Aggregated Results ---")
	if len(allResults) == 0 {
		fmt.Println("No matches found across any nodes.")
	} else {
		for _, res := range allResults {
			fmt.Printf("✅ %s\n", res)
		}
	}
}

// ==========================================
// 4. Main Entry Point
// ==========================================

func main() {
	// Command-line flags to easily switch between Master and Worker
	mode := flag.String("mode", "worker", "Run mode: 'master' or 'worker'")
	port := flag.String("port", "1234", "Port for worker to listen on (e.g., 1234)")
	query := flag.String("query", "go", "Keyword to search for (used in master mode)")
	flag.Parse()

	if *mode == "worker" {
		// Mock different data for different workers to simulate distributed storage
		var localDatabase []string
		if *port == "1234" {
			localDatabase = []string{
				"Go is a compiled programming language.",
				"Python is interpreted and great for AI.",
				"I love building APIs with Go.",
			}
		} else {
			localDatabase = []string{
				"Distributed databases are complex but powerful.",
				"Go concurrency uses goroutines and channels.",
				"Java is widely used in enterprise software.",
			}
		}
		runWorker(*port, localDatabase)

	} else if *mode == "master" {
		// List of all worker nodes in the network
		workers := []string{"localhost:1234", "localhost:1235"}
		runMaster(workers, *query)

	} else {
		log.Fatalf("Unknown mode: %s. Use 'master' or 'worker'", *mode)
	}
}