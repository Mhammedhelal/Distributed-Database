# 🚀 Task 2: Distributed MapReduce in Go

This project implements **Option 1 (MapReduce)** of the Distributed Database Tasks using Go's built-in `net/rpc` package. It simulates a distributed system where data is split across multiple machines (Workers), and a central node (Master) queries them concurrently and aggregates the results.

---

## ✨ Features

* **Distributed Search (Map Phase):** The Master node sends a search keyword to multiple Worker nodes simultaneously. Each Worker searches its own local dataset independently.
* **Result Aggregation (Reduce Phase):** The Master collects the matched records from all Workers and aggregates them into a final result list.
* **Concurrency:** Utilizes Go routines (`goroutines`) and `sync.WaitGroup` to handle multiple RPC calls concurrently, ensuring fast and non-blocking execution.
* **Simulated Distributed Storage:** Different workers load different datasets based on their port numbers to accurately simulate a distributed database environment.

---

## 🛠️ Prerequisites

* [Go (Golang)](https://golang.org/dl/) installed on your machine.
* A terminal or command prompt with support for running multiple windows/tabs simultaneously.

---

## 📂 File Structure

* `mapreduce.go`: A single, cohesive file that contains both the **Master** and **Worker** logic. The execution mode is determined dynamically using command-line flags.

---

## 🚀 How to Run and Test

To fully test the MapReduce flow, you need to simulate a network by opening **three separate terminal windows**.

### Step 1: Start Worker Node 1
Open the first terminal window, navigate to the folder containing `mapreduce.go`, and run:
```bash
go run mapreduce.go -mode worker -port 1234
This starts the first worker on port 1234 and loads its specific dataset.

Step 2: Start Worker Node 2
Open a second terminal window and run:
go run mapreduce.go -mode worker -port 1235
This starts the second worker on port 1235 and loads a different dataset.

Step 3: Run the Master Node (Send the Query)
Open a third terminal window. This terminal acts as the Master node/Client. You can pass any keyword you want to search for using the -query flag.

For example, to search for the word Go:

go run mapreduce.go -mode master -query "Go"

To search for a different word, like complex:

go run mapreduce.go -mode master -query "complex"
📊 Expected Output
When you run the Master node (Step 3) with the query "Go", you should see the following output in your Master terminal:


Master sending query 'Go' to workers: [localhost:1234 localhost:1235]
Received 2 matches from localhost:1234
Received 1 matches from localhost:1235

--- Final Aggregated Results ---
✅ Go is a compiled programming language.
✅ I love building APIs with Go.
✅ Go concurrency uses goroutines and channels.
Meanwhile, in the Worker terminals, you will see logs confirming they received and executed the query:


Executing query for keyword: 'Go'


⚙️ How It Works (Under the Hood)
RPC Registration: When a Worker starts, it registers the Worker struct so its methods can be called remotely over TCP.

Dialing Workers: The Master iterates through a predefined list of worker addresses (localhost:1234, localhost:1235) and establishes an RPC connection with each.

Execution: The Master calls Worker.ExecuteQuery on each connected node.

Thread Safety: When workers return their results, the Master uses a sync.Mutex (mu.Lock()) to safely append the incoming data to the allResults array without data races.