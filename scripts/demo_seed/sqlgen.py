"""SQL rendering helpers: quoting, load-relative times and INSERT statements."""

import json
import uuid

# Fixed namespace so every generated UUID is the same on every run.
_NAMESPACE = uuid.UUID("6f1d2c0a-5b7e-4c1e-9a3d-2f0e8b4c7d10")

RULE = "-- " + "=" * 64


class Raw:
    """A SQL expression written into the statement as it is."""

    def __init__(self, sql: str):
        self.sql = sql

    def __repr__(self) -> str:
        return f"Raw({self.sql!r})"


# The demo visitor's user row. The hub creates it at startup with an ID of its
# own, so rows that must belong to the visitor look the ID up by name.
DEMO_USER = Raw("(SELECT id FROM users WHERE username = 'demo')")

NOW = Raw("NOW()")


def _seconds(days=0.0, hours=0.0, minutes=0.0, seconds=0.0) -> int:
    return int(round(days * 86400 + hours * 3600 + minutes * 60 + seconds))


def ago(days=0.0, hours=0.0, minutes=0.0, seconds=0.0) -> Raw:
    """A time before the load: NOW() - INTERVAL 'N seconds'."""
    total = _seconds(days, hours, minutes, seconds)
    if total <= 0:
        return NOW
    return Raw(f"NOW() - INTERVAL '{total} seconds'")


def ahead(days=0.0, hours=0.0, minutes=0.0, seconds=0.0) -> Raw:
    """A time after the load: NOW() + INTERVAL 'N seconds'."""
    total = _seconds(days, hours, minutes, seconds)
    if total <= 0:
        return NOW
    return Raw(f"NOW() + INTERVAL '{total} seconds'")


def uid(name: str) -> str:
    """A UUID that depends only on its name."""
    return str(uuid.uuid5(_NAMESPACE, name))


def quote(text: str) -> str:
    return "'" + text.replace("'", "''") + "'"


def jsonb(value) -> str:
    return quote(json.dumps(value, separators=(",", ":"), ensure_ascii=False)) + "::jsonb"


def render(value) -> str:
    """One Python value as a SQL literal."""
    if isinstance(value, Raw):
        return value.sql
    if value is None:
        return "NULL"
    if isinstance(value, bool):
        return "TRUE" if value else "FALSE"
    if isinstance(value, (int, float)):
        return repr(value)
    if isinstance(value, str):
        return quote(value)
    if isinstance(value, (dict, list)):
        return jsonb(value)
    raise TypeError(f"cannot render {type(value).__name__} as SQL")


def header(title: str) -> str:
    return f"{RULE}\n-- {title}\n{RULE}\n"


def insert(table: str, columns: list, rows: list, conflict: str = "") -> str:
    """A multi-row INSERT. Each row is a list of Python values, one per column."""
    if not rows:
        return ""
    lines = []
    for row in rows:
        if len(row) != len(columns):
            raise ValueError(f"{table}: row has {len(row)} values for {len(columns)} columns")
        lines.append("    (" + ", ".join(render(value) for value in row) + ")")
    tail = f"\n{conflict};" if conflict else ";"
    return f"INSERT INTO {table} ({', '.join(columns)}) VALUES\n" + ",\n".join(lines) + tail + "\n"


def section(title: str, *statements: str) -> str:
    return header(title) + "\n".join(statement for statement in statements if statement) + "\n"
