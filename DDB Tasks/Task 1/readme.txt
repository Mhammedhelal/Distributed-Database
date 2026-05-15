# 🚀 Distributed Database Tasks - Golang

This project is a Distributed Systems application built using **Go (Golang)**. It focuses on creating a centralized control system (Master/Controller) to manage multiple machines (Workers/Agents) and execute remote tasks over a network using the TCP protocol and JSON for data exchange.

---

## ✨ Features

**Task 1** is fully implemented using a Controller-Agent architecture and includes the following operations:
* 🔒 **Lock Workstation:** Simultaneously lock the screens of all connected devices.
* 🛑 **Remote Shutdown:** Turn off all connected devices remotely.
* 🖼️ **Change Wallpaper:** Change the desktop background of all connected devices by selecting an image on the Controller and broadcasting it.
* 📁 **File Distribution:** Send any file from the Controller to be automatically received and saved on all connected Agent machines.

---

## 🛠️ Prerequisites

* [Go (Golang)](https://golang.org/dl/) version 1.18 or higher installed.
* Windows operating system for the target machines (Agents) to ensure the `shutdown` and Registry (`reg`) commands for wallpaper changes work correctly. *(Note: The code can be easily modified to support Linux commands).*

---

## 📂 Project Structure

The project consists of two main files:
1. `controller.go`: Acts as the Server (Master Node) that accepts incoming connections and broadcasts commands.
2. `agent.go`: Acts as the Client (Worker Node) that runs in the background on the target machines to receive and execute commands.

---

## 🚀 How to Run

### 1. Run the Controller (Master Node)
Open your terminal in the project directory and run the following command:
```bash
go run controller.go -port 9000