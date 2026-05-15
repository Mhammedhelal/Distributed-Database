"""Replication endpoint — accepts WAL entries from master only."""
from fastapi import APIRouter, Depends
from pydantic import BaseModel
from typing import Any
from app.auth import require_master_token
from app.db.engine import exec_sql, use_db
import os

router = APIRouter()
NODE_ID = os.getenv("NODE_ID", "3")


class WALEntry(BaseModel):
    seq: int
    op: str
    database: str = ""
    table: str = ""
    data: dict[str, Any] | None = None
    where: str | None = None
    where_args: list[Any] | None = None
    cols: list[dict] | None = None


def _escape(val: Any) -> str:
    """Minimal SQL escaping for string values."""
    if val is None:
        return "NULL"
    s = str(val).replace("\\", "\\\\").replace("'", "\\'")
    return f"'{s}'"


def _apply(entry: WALEntry):
    op = entry.op
    db = entry.database
    tbl = entry.table

    if db:
        exec_sql(f"CREATE DATABASE IF NOT EXISTS `{db}`")
        exec_sql(f"USE `{db}`")

    if op == "CREATE_DB":
        pass  # handled above

    elif op == "DROP_DB":
        exec_sql(f"DROP DATABASE IF EXISTS `{db}`")

    elif op == "CREATE_TABLE":
        col_defs = "`id` INT AUTO_INCREMENT PRIMARY KEY"
        for c in (entry.cols or []):
            col_defs += f", `{c['name']}` {c['type']}"
        exec_sql(f"CREATE TABLE IF NOT EXISTS `{tbl}` ({col_defs})")

    elif op == "DROP_TABLE":
        exec_sql(f"DROP TABLE IF EXISTS `{tbl}`")

    elif op == "INSERT":
        data = entry.data or {}
        cols = ", ".join(f"`{k}`" for k in data)
        vals = ", ".join(_escape(v) for v in data.values())
        exec_sql(f"INSERT INTO `{tbl}` ({cols}) VALUES ({vals})")

    elif op == "UPDATE":
        data = entry.data or {}
        set_clause = ", ".join(f"`{k}`={_escape(v)}" for k, v in data.items())
        sql = f"UPDATE `{tbl}` SET {set_clause}"
        if entry.where:
            where = entry.where
            if entry.where_args:
                where = where.replace("?", _escape(entry.where_args[0]), 1)
            sql += f" WHERE {where}"
        exec_sql(sql)

    elif op == "DELETE":
        sql = f"DELETE FROM `{tbl}`"
        if entry.where:
            where = entry.where
            if entry.where_args:
                where = where.replace("?", _escape(entry.where_args[0]), 1)
            sql += f" WHERE {where}"
        exec_sql(sql)

    else:
        raise ValueError(f"unknown op: {op}")


@router.post("/replication/apply", dependencies=[Depends(require_master_token)])
async def apply_entry(entry: WALEntry):
    try:
        _apply(entry)
    except Exception as e:
        return {"ok": False, "node_id": NODE_ID, "seq": entry.seq, "error": str(e)}
    return {"ok": True, "node_id": NODE_ID, "seq": entry.seq}


@router.post("/internal/catchup", dependencies=[Depends(require_master_token)])
async def catchup(entries: list[WALEntry]):
    for entry in entries:
        try:
            _apply(entry)
        except Exception as e:
            return {"ok": False, "failed_at_seq": entry.seq, "error": str(e)}
    return {"ok": True, "applied": len(entries)}