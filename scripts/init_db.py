#!/usr/bin/env python3
"""
Initialize a fresh MySQL database for aibbs and import schema using PyMySQL.

Sources DB settings from config/config.json (nested database section) with environment overrides:
  - Prefer DATABASE_URI like: user:pass@tcp(host:port)/dbname?params
  - Otherwise use DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME

Usage:
  python3 scripts/init_db.py
"""
from __future__ import annotations

import json
import os
import re
import sys
from pathlib import Path

try:
    import pymysql
    from pymysql import OperationalError
except ImportError:
    print("[ERROR] PyMySQL is not installed. Install it with: pip install PyMySQL", file=sys.stderr)
    sys.exit(1)


REPO_ROOT = Path(__file__).resolve().parent.parent
CONFIG_PATH = REPO_ROOT / "config" / "config.json"
INIT_SQL_PATH = REPO_ROOT / "scripts" / "init.sql"


def load_config() -> dict:
    if not CONFIG_PATH.exists():
        return {}
    with CONFIG_PATH.open("r", encoding="utf-8") as f:
        return json.load(f)


def get_db_settings(cfg: dict) -> dict:
    # Read from JSON (nested or flat)
    db = (cfg.get("database") or {}) if isinstance(cfg, dict) else {}
    out = {
        "host": str(db.get("DBHost") or cfg.get("DBHost") or ""),
        "port": str(db.get("DBPort") or cfg.get("DBPort") or ""),
        "user": str(db.get("DBUser") or cfg.get("DBUser") or ""),
        "password": str(db.get("DBPassword") or cfg.get("DBPassword") or ""),
        "name": str(db.get("DBName") or cfg.get("DBName") or ""),
        "dsn": str(db.get("DatabaseURI") or cfg.get("DatabaseURI") or ""),
    }

    # Env overrides
    env = os.environ
    if env.get("DATABASE_URI"):
        out["dsn"] = env["DATABASE_URI"]
    for k_env, k_out in [
        ("DB_HOST", "host"),
        ("DB_PORT", "port"),
        ("DB_USER", "user"),
        ("DB_PASSWORD", "password"),
        ("DB_NAME", "name"),
    ]:
        if env.get(k_env):
            out[k_out] = env[k_env]

    # If DSN present, try parse it
    if out["dsn"]:
        parsed = parse_mysql_dsn(out["dsn"])  # may raise ValueError
        out.update(parsed)
    return out


def parse_mysql_dsn(dsn: str) -> dict:
    """
    Parse MySQL DSN of the form: user:password@tcp(host:port)/dbname?params
    Note: If password contains '@' or ':', parsing is ambiguous. Prefer discrete DB_* vars in that case.
    """
    m = re.match(r"^(?P<user>[^:]+):(?P<pw>[^@]*)@tcp\((?P<host>[^\)]+)\)/(?P<db>[^\?]+)(?:\?(?P<qs>.*))?$", dsn)
    if not m:
        raise ValueError(f"Unsupported DATABASE_URI format: {dsn}")
    hostport = m.group("host")
    host, _, port = hostport.partition(":")
    return {
        "user": m.group("user"),
        "password": m.group("pw"),
        "host": host,
        "port": port or "3306",
        "name": m.group("db"),
        "dsn": dsn,
    }


def ensure_database(conn_params: dict) -> None:
    host = conn_params["host"] or "127.0.0.1"
    port = int(conn_params["port"] or 3306)
    user = conn_params["user"]
    password = conn_params.get("password") or ""
    name = conn_params["name"]

    if not (host and port and user and name):
        raise SystemExit("[ERROR] Missing one of required DB settings: host/port/user/name")

    print(f"[INFO] Connecting to MySQL {host}:{port} as {user} (no db)...")
    conn = pymysql.connect(
        host=host,
        port=port,
        user=user,
        password=password,
        autocommit=True,
        connect_timeout=5,
        charset="utf8mb4",
    )
    try:
        with conn.cursor() as cur:
            sql = f"CREATE DATABASE IF NOT EXISTS `{name}` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
            cur.execute(sql)
        print(f"[OK] Database '{name}' ensured.")
    finally:
        conn.close()

    print(f"[INFO] Connecting to MySQL {host}:{port}/{name} ...")
    conn2 = pymysql.connect(
        host=host,
        port=port,
        user=user,
        password=password,
        database=name,
        autocommit=True,
        connect_timeout=5,
        charset="utf8mb4",
    )
    try:
        run_init_sql(conn2)
    finally:
        conn2.close()


def run_init_sql(conn) -> None:
    if not INIT_SQL_PATH.exists():
        raise SystemExit(f"[ERROR] SQL file not found: {INIT_SQL_PATH}")
    print(f"[INFO] Importing schema from {INIT_SQL_PATH} ...")
    sql = INIT_SQL_PATH.read_text(encoding="utf-8")
    statements = list(split_sql_statements(sql))
    with conn.cursor() as cur:
        for stmt in statements:
            cur.execute(stmt)
    print("[OK] Schema import finished.")


def split_sql_statements(sql: str):
    """A simple splitter for statements separated by semicolons; ignores -- comments and blank lines."""
    buf = []
    for raw_line in sql.splitlines():
        line = raw_line.strip()
        if not line or line.startswith("--"):
            continue
        buf.append(line)
        if line.endswith(";"):
            stmt = " ".join(buf).rstrip(";")
            if stmt:
                yield stmt
            buf = []
    # Any trailing part without semicolon (unlikely) - ignore or execute
    if buf:
        stmt = " ".join(buf)
        if stmt:
            yield stmt


def main():
    try:
        cfg = load_config()
        params = get_db_settings(cfg)
        # Mask password when echoing
        safe_params = {**params, "password": "***" if params.get("password") else ""}
        print(f"[INFO] Using DB settings: host={safe_params.get('host')} port={safe_params.get('port')} user={safe_params.get('user')} name={safe_params.get('name')}")
        ensure_database(params)
        print("[DONE] Database initialization completed.")
    except OperationalError as e:
        print(f"[ERROR] MySQL connection/operation failed: {e}", file=sys.stderr)
        sys.exit(2)
    except ValueError as e:
        print(f"[ERROR] {e}", file=sys.stderr)
        sys.exit(3)
    except FileNotFoundError as e:
        print(f"[ERROR] {e}", file=sys.stderr)
        sys.exit(4)
    except Exception as e:
        print(f"[ERROR] Unexpected error: {e}", file=sys.stderr)
        sys.exit(99)


if __name__ == "__main__":
    main()
