# Distributed-Database

A distributed database system with multiple components including an API Gateway, Master Server, Worker Nodes, and a GUI dashboard.

## Project Structure

```
├── api-gateway/        # REST API Gateway with rate limiting and authentication
├── master/             # Master server for query coordination
├── worker-node1/       # C++ worker node with analytics
├── worker-node2/       # Python worker node with replication support
└── gui/                # Streamlit-based web dashboard
```

## Components

### API Gateway

- **Language**: Go
- **Features**:
  - Request routing and proxying
  - HMAC-based authentication
  - Rate limiting (token bucket algorithm)
  - Health check proxy
- **Location**: [api-gateway/](api-gateway/)

### Master Server

- **Language**: Go
- **Features**:
  - Query parsing and execution
  - Authentication middleware
  - Database store management
  - Query coordination
- **Location**: [master/](master/)

### Worker Node 1

- **Language**: C++
- **Features**:
  - MySQL client integration
  - Analytics processing
  - Crow framework for HTTP
- **Location**: [worker-node1/](worker-node1/)

### Worker Node 2

- **Language**: Python
- **Features**:
  - Query processing routes
  - Replication support
  - Search capabilities
  - Database engine
- **Location**: [worker-node2/](worker-node2/)

### GUI Dashboard

- **Language**: Python (Streamlit)
- **Features**:
  - Web-based user interface
  - Real-time monitoring
- **Location**: [gui/](gui/)

## Prerequisites

- Go 1.16+
- Python 3.8+
- C++ 11 (for worker-node1)
- Docker (optional)
- MySQL (for worker nodes)

## Installation & Setup

### 1. Clone the Repository

```bash
git clone <repository-url>
cd Distributed-Database
```

### 2. API Gateway

```bash
cd api-gateway
# Build
go build -o gateway cmd/gateway/main.go

# Run
./gateway
```

### 3. Master Server

```bash
cd master
# Download dependencies
go mod download

# Run
go run cmd/server/main.go
```

### 4. Worker Node 1 (C++)

```bash
cd worker-node1
mkdir build && cd build
cmake ..
make

# Run
./worker_node
```

### 5. Worker Node 2 (Python)

```bash
cd worker-node2
pip install -r requirements.txt
python app/main.py
```

### 6. GUI Dashboard

```bash
cd gui
pip install -r requirements.txt
streamlit run app.py
```

## Configuration

- **API Gateway**: [api-gateway/config/gateway.yaml](api-gateway/config/gateway.yaml)
- **Master Server**: [master/config/master.json](master/config/master.json)

## Docker Deployment

Each component includes a Dockerfile for containerization:

```bash
# Build images
docker build -t api-gateway ./api-gateway
docker build -t worker-node1 ./worker-node1
docker build -t worker-node2 ./worker-node2

# Run containers
docker run -p 8080:8080 api-gateway
docker run -p 8081:8081 worker-node1
docker run -p 8082:8082 worker-node2
```

## API Endpoints

The API Gateway provides RESTful endpoints for database operations. See [api-gateway/Readme.md](api-gateway/Readme.md) for detailed API documentation.

## Testing

Run tests in each component:

```bash
# API Gateway
cd api-gateway && go test ./...

# Master Server
cd master && go test ./...

# Worker Node 1 (C++)
cd worker-node1 && ctest

# Worker Node 2
cd worker-node2 && pytest
```

## Architecture

The system follows a distributed architecture:

1. **Client** → **API Gateway** (authentication, routing)
2. **API Gateway** → **Master Server** (query coordination)
3. **Master Server** → **Worker Nodes** (query execution)
4. **Worker Nodes** → **Database** (data storage and retrieval)

## Contributing

1. Create a feature branch
2. Make your changes
3. Run tests
4. Submit a pull request

## License

[Add your license information here]

## Contact

For questions or issues, please open an issue on the repository.
