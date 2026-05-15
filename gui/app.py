"""
Distributed Database — Streamlit GUI
=====================================
Pages:
  1. Query Editor   — run any SQL against the cluster
  2. Table Browser  — browse databases and tables
  3. Cluster Health — live node status
  4. Admin          — create / drop databases (master only)
"""
import streamlit as st
import requests
import pandas as pd
import json
import os

# ── Config ────────────────────────────────────────────────────────────────────
GATEWAY_URL = os.getenv("GATEWAY_URL", "http://localhost:8000")
API_KEY     = os.getenv("GATEWAY_API_KEY", "")   # optional bearer token

st.set_page_config(
    page_title="Distributed DB",
    page_icon="🗄️",
    layout="wide",
    initial_sidebar_state="expanded",
)

# ── Session state defaults ────────────────────────────────────────────────────
if "query_history" not in st.session_state:
    st.session_state.query_history = []
if "last_result" not in st.session_state:
    st.session_state.last_result = None


# ── HTTP helpers ──────────────────────────────────────────────────────────────
def _headers():
    h = {"Content-Type": "application/json"}
    if API_KEY:
        h["Authorization"] = f"Bearer {API_KEY}"
    return h


def run_query(sql: str) -> dict:
    try:
        resp = requests.post(
            f"{GATEWAY_URL}/query",
            json={"sql": sql},
            headers=_headers(),
            timeout=10,
        )
        return resp.json()
    except requests.exceptions.ConnectionError:
        return {"error": f"Cannot connect to gateway at {GATEWAY_URL}"}
    except Exception as e:
        return {"error": str(e)}


def cluster_status() -> dict:
    try:
        resp = requests.get(f"{GATEWAY_URL}/cluster/status", headers=_headers(), timeout=5)
        return resp.json()
    except Exception as e:
        return {"error": str(e)}


def gateway_health() -> dict:
    try:
        resp = requests.get(f"{GATEWAY_URL}/health", headers=_headers(), timeout=3)
        return resp.json()
    except Exception as e:
        return {"error": str(e)}


# ── Sidebar ───────────────────────────────────────────────────────────────────
with st.sidebar:
    st.title("🗄️ Distributed DB")
    st.caption(f"Gateway: `{GATEWAY_URL}`")

    page = st.radio(
        "Navigate",
        ["Query Editor", "Table Browser", "Cluster Health", "Admin"],
        index=0,
    )

    st.divider()
    if st.button("🔄 Check Gateway"):
        h = gateway_health()
        if "error" in h:
            st.error(h["error"])
        else:
            st.success(f"Gateway OK — WAL seq {h.get('wal_seq', '?')}")


# ══════════════════════════════════════════════════════════════════════════════
# Page 1: Query Editor
# ══════════════════════════════════════════════════════════════════════════════
if page == "Query Editor":
    st.header("📝 Query Editor")

    col1, col2 = st.columns([3, 1])
    with col1:
        sql = st.text_area(
            "SQL Statement",
            height=160,
            placeholder="SELECT * FROM users WHERE id = 1",
            help="Writes (INSERT/UPDATE/DELETE/CREATE/DROP) are routed to master. "
                 "SELECT is load-balanced across workers.",
        )
    with col2:
        st.markdown("**Quick templates**")
        templates = {
            "SELECT all":   "SELECT * FROM <table>",
            "INSERT row":   "INSERT INTO <table> (col1, col2) VALUES ('a', 'b')",
            "UPDATE row":   "UPDATE <table> SET col1='val' WHERE id = 1",
            "DELETE row":   "DELETE FROM <table> WHERE id = 1",
            "CREATE TABLE": "CREATE TABLE users (name VARCHAR(100), email VARCHAR(255))",
        }
        for label, tmpl in templates.items():
            if st.button(label, use_container_width=True):
                st.session_state["_tmpl"] = tmpl
                st.rerun()

    if "_tmpl" in st.session_state:
        sql = st.session_state.pop("_tmpl")

    run_col, clear_col = st.columns([1, 5])
    with run_col:
        run = st.button("▶ Run", type="primary", use_container_width=True)
    with clear_col:
        if st.button("🗑 Clear history", use_container_width=False):
            st.session_state.query_history = []

    if run and sql.strip():
        with st.spinner("Executing…"):
            result = run_query(sql.strip())
        st.session_state.last_result = result
        st.session_state.query_history.insert(0, {"sql": sql.strip(), "result": result})

    result = st.session_state.last_result
    if result:
        if "error" in result:
            st.error(f"**Error:** {result['error']}")
        else:
            meta_cols = st.columns(3)
            if "wal_seq" in result:
                meta_cols[0].metric("WAL Seq", result["wal_seq"])
            if "affected_rows" in result:
                meta_cols[1].metric("Affected Rows", result["affected_rows"])
            if "last_insert_id" in result and result["last_insert_id"]:
                meta_cols[2].metric("Last Insert ID", result["last_insert_id"])

            if result.get("message"):
                st.success(result["message"])

            rows = result.get("rows")
            if rows:
                st.dataframe(pd.DataFrame(rows), use_container_width=True)
            elif "affected_rows" in result and not result.get("message"):
                st.success(f"{result['affected_rows']} row(s) affected")

            acks = result.get("replica_acks")
            if acks:
                with st.expander("Replication ACKs"):
                    st.json(acks)

    if st.session_state.query_history:
        st.divider()
        st.subheader("History")
        for i, item in enumerate(st.session_state.query_history[:10]):
            with st.expander(f"`{item['sql'][:80]}`", expanded=(i == 0)):
                r = item["result"]
                if "error" in r:
                    st.error(r["error"])
                elif r.get("rows"):
                    st.dataframe(pd.DataFrame(r["rows"]), use_container_width=True)
                else:
                    st.json(r)


# ══════════════════════════════════════════════════════════════════════════════
# Page 2: Table Browser
# ══════════════════════════════════════════════════════════════════════════════
elif page == "Table Browser":
    st.header("📂 Table Browser")

    col1, col2 = st.columns([2, 3])

    with col1:
        db_name = st.text_input("Database name", placeholder="distdb")
        if st.button("List tables", type="primary"):
            if not db_name.strip():
                st.warning("Enter a database name")
            else:
                res = run_query(f"SHOW TABLES")
                if "error" in res:
                    st.error(res["error"])
                elif res.get("rows"):
                    tables = [list(r.values())[0] for r in res["rows"]]
                    st.session_state["_tables"] = tables
                    st.session_state["_db"] = db_name
                else:
                    st.info("No tables found")

        tables = st.session_state.get("_tables", [])
        if tables:
            chosen = st.selectbox("Tables", tables)
            st.session_state["_chosen_table"] = chosen

    with col2:
        chosen = st.session_state.get("_chosen_table")
        if chosen:
            st.subheader(f"Preview: `{chosen}`")
            tab1, tab2 = st.tabs(["Data (first 50)", "Schema"])
            with tab1:
                res = run_query(f"SELECT * FROM `{chosen}` LIMIT 50")
                if "error" in res:
                    st.error(res["error"])
                elif res.get("rows"):
                    st.dataframe(pd.DataFrame(res["rows"]), use_container_width=True)
                else:
                    st.info("Table is empty")
            with tab2:
                res = run_query(f"DESCRIBE `{chosen}`")
                if "error" in res:
                    st.error(res["error"])
                elif res.get("rows"):
                    st.dataframe(pd.DataFrame(res["rows"]), use_container_width=True)


# ══════════════════════════════════════════════════════════════════════════════
# Page 3: Cluster Health
# ══════════════════════════════════════════════════════════════════════════════
elif page == "Cluster Health":
    st.header("🩺 Cluster Health")

    if st.button("🔄 Refresh", type="primary"):
        st.rerun()

    status = cluster_status()

    if "error" in status:
        st.error(f"Cannot reach gateway: {status['error']}")
    else:
        nodes = status.get("nodes", [])
        if not nodes:
            st.warning("No nodes registered yet")
        else:
            alive = sum(1 for n in nodes if n.get("status") == "alive")
            down  = len(nodes) - alive

            c1, c2, c3 = st.columns(3)
            c1.metric("Total Nodes", len(nodes))
            c2.metric("Alive", alive, delta=None)
            c3.metric("Down", down, delta=None)

            st.divider()
            for node in nodes:
                is_alive = node.get("status") == "alive"
                icon = "🟢" if is_alive else "🔴"
                role_badge = "**[MASTER]**" if node.get("role") == "master" else "[worker]"

                with st.container(border=True):
                    cols = st.columns([1, 2, 2, 2, 2])
                    cols[0].markdown(icon)
                    cols[1].markdown(f"**{node.get('id', '?')}** {role_badge}")
                    cols[2].code(node.get("address", ""))
                    cols[3].markdown(f"Status: `{node.get('status', '?')}`")
                    cols[4].markdown(f"WAL ACK: `{node.get('last_seq_ack', 0)}`")


# ══════════════════════════════════════════════════════════════════════════════
# Page 4: Admin
# ══════════════════════════════════════════════════════════════════════════════
elif page == "Admin":
    st.header("⚙️ Admin — Master Only")
    st.warning("Operations on this page are routed directly to the master node.", icon="⚠️")

    tab1, tab2, tab3 = st.tabs(["Create Database", "Drop Database", "Create Table"])

    with tab1:
        st.subheader("Create Database")
        new_db = st.text_input("Database name", key="create_db_name")
        if st.button("Create", type="primary", key="btn_create_db"):
            if not new_db.strip():
                st.warning("Enter a name")
            else:
                res = run_query(f"CREATE DATABASE {new_db.strip()}")
                if "error" in res:
                    st.error(res["error"])
                else:
                    st.success(res.get("message", "Done"))

    with tab2:
        st.subheader("Drop Database")
        st.error("This is irreversible. Only the master node will accept this request.", icon="🗑️")
        drop_db = st.text_input("Database name to drop", key="drop_db_name")
        confirm = st.checkbox("I confirm I want to permanently drop this database")
        if st.button("Drop Database", type="primary", disabled=not confirm, key="btn_drop_db"):
            res = run_query(f"DROP DATABASE {drop_db.strip()}")
            if "error" in res:
                st.error(res["error"])
            else:
                st.success(res.get("message", "Dropped"))

    with tab3:
        st.subheader("Create Table")
        tbl_db = st.text_input("Database", key="ct_db")
        tbl_name = st.text_input("Table name", key="ct_name")
        st.caption("Define columns (one per line): `column_name TYPE` e.g. `name VARCHAR(100)`")
        col_defs_raw = st.text_area("Columns", height=120, key="ct_cols",
                                    placeholder="name VARCHAR(100)\nemail VARCHAR(255)\nage INT")
        if st.button("Create Table", type="primary", key="btn_create_table"):
            if not tbl_name.strip() or not col_defs_raw.strip():
                st.warning("Fill in table name and at least one column")
            else:
                lines = [l.strip() for l in col_defs_raw.strip().splitlines() if l.strip()]
                col_sql = ", ".join(lines)
                sql = f"CREATE TABLE {tbl_name.strip()} ({col_sql})"
                if tbl_db.strip():
                    run_query(f"CREATE DATABASE IF NOT EXISTS {tbl_db.strip()}")
                res = run_query(sql)
                if "error" in res:
                    st.error(res["error"])
                else:
                    st.success(res.get("message", "Table created"))