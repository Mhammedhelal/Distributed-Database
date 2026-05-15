"""Worker Node 2 — Python / FastAPI distributed database worker."""
import os
from fastapi import FastAPI
from fastapi.responses import JSONResponse
from app.routes import query, replication, search

NODE_ID = os.getenv("NODE_ID", "3")

app = FastAPI(
    title="Distributed DB — Worker Node 2 (Python)",
    description="FastAPI worker node with full-text search capability.",
    version="1.0.0",
)

# ── Routers ───────────────────────────────────────────────────────────────────
app.include_router(query.router)
app.include_router(replication.router)
app.include_router(search.router)


# ── Health ────────────────────────────────────────────────────────────────────
@app.get("/health")
async def health():
    return {"status": "ok", "service": "worker-node2-python", "node_id": NODE_ID}


# ── Global exception handler ─────────────────────────────────────────────────
@app.exception_handler(Exception)
async def global_exception_handler(request, exc):
    return JSONResponse(status_code=500, content={"error": str(exc)})