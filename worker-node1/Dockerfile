# ── Stage 1: build ────────────────────────────────────────────────────────────
FROM ubuntu:24.04 AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential cmake git curl ca-certificates \
    libboost-system-dev libboost-dev \
    libmysqlclient-dev libssl-dev \
    nlohmann-json3-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

# Download Crow single-header (v1.1.0)
RUN curl -L https://github.com/CrowCpp/Crow/releases/download/v1.1.0/crow_all.h \
    -o /src/include/crow_all.h

COPY CMakeLists.txt .
COPY src/ src/
COPY include/ include/

RUN cmake -B build -DCMAKE_BUILD_TYPE=Release && cmake --build build -j$(nproc)

# ── Stage 2: runtime ──────────────────────────────────────────────────────────
FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    libmysqlclient21 libssl3 \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /src/build/worker_node1 /usr/local/bin/worker_node1

EXPOSE 8081

ENV MYSQL_HOST=mysql-worker1 \
    MYSQL_PORT=3306 \
    MYSQL_USER=root \
    MYSQL_PASS=rootpass \
    MYSQL_DB=distdb \
    HMAC_SECRET=change-me-in-production \
    PORT=8081 \
    NODE_ID=2

ENTRYPOINT ["/usr/local/bin/worker_node1"]