"""Full-text search endpoint — Python worker-2 special capability.

Uses MySQL FULLTEXT indexes (MATCH … AGAINST) to provide ranked
full-text search across any table. Falls back to LIKE if no FULLTEXT
index exists on the requested column.
"""
from fastapi import APIRouter, Depends, HTTPException
from pydantic import BaseModel
from app.auth import require_master_token
from app.db.engine import exec_sql, use_db

router = APIRouter()


class SearchRequest(BaseModel):
    database: str
    table: str
    column: str
    query: str
    limit: int = 20


@router.post("/search", dependencies=[Depends(require_master_token)])
async def full_text_search(req: SearchRequest):
    """
    Performs MATCH ... AGAINST full-text search.
    Requires a FULLTEXT index on the column; falls back to LIKE otherwise.
    """
    if not req.query.strip():
        raise HTTPException(status_code=400, detail="query must not be empty")

    use_db(req.database)

    # Check if a FULLTEXT index exists for this column
    idx_rows = exec_sql(
        "SELECT INDEX_TYPE FROM information_schema.STATISTICS "
        "WHERE TABLE_SCHEMA = :db AND TABLE_NAME = :tbl AND COLUMN_NAME = :col "
        "AND INDEX_TYPE = 'FULLTEXT'",
        {"db": req.database, "tbl": req.table, "col": req.column}
    )

    try:
        if idx_rows:
            # FULLTEXT search with relevance score
            safe_query = req.query.replace("'", "\\'")
            sql = (
                f"SELECT *, MATCH(`{req.column}`) AGAINST ('{safe_query}' IN BOOLEAN MODE) "
                f"AS _relevance "
                f"FROM `{req.table}` "
                f"WHERE MATCH(`{req.column}`) AGAINST ('{safe_query}' IN BOOLEAN MODE) "
                f"ORDER BY _relevance DESC "
                f"LIMIT {req.limit}"
            )
        else:
            # Fallback: LIKE search
            safe_query = req.query.replace("'", "\\'").replace("%", "\\%")
            sql = (
                f"SELECT * FROM `{req.table}` "
                f"WHERE `{req.column}` LIKE '%{safe_query}%' "
                f"LIMIT {req.limit}"
            )

        rows = exec_sql(sql) or []
        return {
            "mode": "fulltext" if idx_rows else "like_fallback",
            "query": req.query,
            "count": len(rows),
            "results": rows,
        }
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.post("/search/create-index", dependencies=[Depends(require_master_token)])
async def create_fulltext_index(database: str, table: str, column: str):
    """Creates a FULLTEXT index on a column to enable ranked search."""
    use_db(database)
    try:
        exec_sql(
            f"ALTER TABLE `{table}` ADD FULLTEXT INDEX ft_{column} (`{column}`)"
        )
        return {"ok": True, "message": f"FULLTEXT index created on {table}.{column}"}
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))