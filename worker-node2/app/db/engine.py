"""Database engine and session factory for worker-node2."""
import os
from sqlalchemy import create_engine, text
from sqlalchemy.orm import sessionmaker

_engine = None
_SessionLocal = None


def get_engine():
    global _engine
    if _engine is None:
        url = os.getenv(
            "MYSQL_URL",
            "mysql+pymysql://root:rootpass@mysql-worker2:3306/distdb"
        )
        _engine = create_engine(
            url,
            pool_pre_ping=True,
            pool_size=10,
            max_overflow=20,
            echo=False,
        )
    return _engine


def get_session():
    global _SessionLocal
    if _SessionLocal is None:
        _SessionLocal = sessionmaker(bind=get_engine(), autocommit=False, autoflush=False)
    return _SessionLocal()


def exec_sql(sql: str, params: dict | None = None):
    """Execute a raw SQL statement; returns rows for SELECT, None otherwise."""
    engine = get_engine()
    with engine.connect() as conn:
        result = conn.execute(text(sql), params or {})
        conn.commit()
        if result.returns_rows:
            cols = list(result.keys())
            return [dict(zip(cols, row)) for row in result.fetchall()]
        return None


def use_db(database: str):
    """Switch active schema, creating it if needed."""
    engine = get_engine()
    with engine.connect() as conn:
        conn.execute(text(f"CREATE DATABASE IF NOT EXISTS `{database}`"))
        conn.execute(text(f"USE `{database}`"))
        conn.commit()