"""Query endpoint — SELECT open to all, write ops require master token."""
from fastapi import APIRouter, Request, HTTPException, Depends
from pydantic import BaseModel
from app.db.engine import exec_sql, use_db
from app.auth import require_master_token, validate_master_token, MASTER_TOKEN_HEADER

router = APIRouter()

WRITE_KEYWORDS = {"INSERT", "UPDATE", "DELETE", "CREATE", "DROP", "ALTER"}


class QueryRequest(BaseModel):
    sql: str
    database: str | None = None


@router.post("/query")
async def run_query(req: QueryRequest, request: Request):
    first_word = req.sql.strip().split()[0].upper() if req.sql.strip() else ""

    # Enforce slave security: writes only accepted from master
    if first_word in WRITE_KEYWORDS:
        token = request.headers.get(MASTER_TOKEN_HEADER, "")
        if not token or not validate_master_token(token):
            raise HTTPException(
                status_code=403,
                detail="slaves accept INSERT/UPDATE/DELETE only from master"
            )

    if req.database:
        use_db(req.database)

    try:
        rows = exec_sql(req.sql)
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

    if rows is None:
        return {"affected_rows": 0, "message": "ok"}
    return {"rows": rows}