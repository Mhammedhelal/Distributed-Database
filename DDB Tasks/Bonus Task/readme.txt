# 🚀 Bonus Task: Cache & Main Server Architecture in Go

This project implements the bonus requirement of the Distributed Database Tasks. It simulates a data pipeline where a Client sends data to a Cache server. The Cache temporarily stores the data, forwards it to a Main permanent server, and then automatically deletes the data from its own memory once the transfer is successful.

---

## ✨ Features

* **Temporary Storage (Cache):** The Cache server safely stores incoming data using an in-memory map and a Mutex (`sync.Mutex`) to handle concurrent requests.
* **Data Forwarding:** The Cache acts as both an RPC Server (for the client) and an RPC Client (for the Main Server), seamlessly forwarding data.
* **Auto-Cleanup:** Once the Main server confirms the data is saved, the Cache server immediately deletes the data from its local storage, ensuring memory efficiency.

---

## 🛠️ Prerequisites

* [Go (Golang)](https://golang.org/dl/) installed on your machine.
* A terminal or command prompt with support for running multiple windows/tabs.

---

## 🚀 How to Run and Test

You will need **three separate terminal windows** to simulate the Client, the Cache Server, and the Main Server.

### Step 1: Start the Main Server
Open the first terminal window, navigate to the folder containing `bonus.go`, and run:
```bash
go run bonus.go -mode main

This starts the Main Server on port 5001.

Step 2: Start the Cache Server
Open a second terminal window and run:
go run bonus.go -mode cache

This starts the Cache Server on port 5000.

Step 3: Run the Client (Send Data)
Open a third terminal window. This terminal acts as the Client. You can send any custom data using the -data flag:

go run bonus.go -mode client -data "User login record #9921"

go run bonus.go -mode client -data "User login record #9921"
[Client] 📤 Sending data to Cache Server: 'User login record #9921'
[Client] ✅ Response: Success: Data saved in Main and cleared from Cache.

2. On the Cache Server Terminal:
[Cache Server] 🟡 Data temporarily stored with key: key_1684538192...
[Cache Server] ⏳ Forwarding data to Main Server...
[Cache Server] 🔴 Data successfully forwarded and deleted from Cache.

3. On the Main Server Terminal:
[Main Server] 💾 Data received and saved permanently: 'User login record #9921'