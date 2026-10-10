#!/usr/bin/env python3
"""Run a PostgreSQL client with DATABASE_URL supplied privately on descriptor 3."""
import contextlib
import ctypes
import ctypes.util
import os
from pathlib import Path
import shutil
import signal
import subprocess
import sys
import tempfile


class ConninfoOption(ctypes.Structure):
    _fields_ = [(name, ctypes.c_char_p) for name in
                ("keyword", "envvar", "compiled", "val", "label", "dispchar")]
    _fields_.append(("dispsize", ctypes.c_int))


def load_libpq(client="pg_dump"):
    """Use the installed client's library, including keg-only macOS installs."""
    executable = shutil.which(client)
    candidates = []
    if executable:
        libdir = Path(executable).resolve().parent.parent / "lib"
        candidates.extend(str(libdir / name) for name in
                          ("libpq.so.5", "libpq.dylib", "libpq.so")
                          if (libdir / name).is_file())
    system_library = ctypes.util.find_library("pq")
    if system_library:
        candidates.append(system_library)
    for candidate in candidates:
        try:
            library = ctypes.CDLL(candidate)
            break
        except OSError:
            continue
    else:
        raise ValueError("PostgreSQL client library libpq is required")
    pointer = ctypes.POINTER(ConninfoOption)
    library.PQconninfoParse.argtypes = [ctypes.c_char_p, ctypes.POINTER(ctypes.c_void_p)]
    library.PQconninfoParse.restype = pointer
    library.PQconndefaults.argtypes = []
    library.PQconndefaults.restype = pointer
    library.PQconninfoFree.argtypes = [pointer]
    library.PQconninfoFree.restype = None
    library.PQfreemem.argtypes = [ctypes.c_void_p]
    library.PQfreemem.restype = None
    return library


def parse_connection(library, connection):
    if b"\0" in connection:
        raise ValueError("invalid DATABASE_URL")
    error = ctypes.c_void_p()
    options = library.PQconninfoParse(connection, ctypes.byref(error))
    if not options:
        if error.value:
            library.PQfreemem(error)
        # libpq's diagnostic can repeat credential-bearing input.
        raise ValueError("invalid DATABASE_URL; connection details hidden")
    result = {}
    try:
        index = 0
        while options[index].keyword:
            option = options[index]
            if option.val is not None:
                result[option.keyword.decode("ascii")] = option.val
            index += 1
    finally:
        library.PQconninfoFree(options)
    return result


def private_file(path, payload):
    with os.fdopen(os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), "wb") as handle:
        handle.write(payload)


@contextlib.contextmanager
def connection_environment(connection, client):
    options = parse_connection(load_libpq(client), connection)
    if "service" in options:
        raise ValueError("DATABASE_URL must specify connection values, not a nested service")
    password = options.pop("password", None)
    if password is None and "PGPASSWORD" in os.environ:
        password = os.fsencode(os.environ["PGPASSWORD"])
    env = dict(os.environ)
    for key in ("DATABASE_URL", "database_url", "PGDATABASE", "PGPASSWORD"):
        env.pop(key, None)
    with tempfile.TemporaryDirectory(prefix="labtether-db.") as temporary:
        directory = Path(temporary)
        if password is not None:
            if b"\n" in password or b"\r" in password or b"\0" in password:
                raise ValueError("database password contains unsupported control characters")
            passfile = directory / "pgpass"
            escaped = password.replace(b"\\", b"\\\\").replace(b":", b"\\:")
            private_file(passfile, b"*:*:*:*:" + escaped + b"\n")
            options["passfile"] = os.fsencode(passfile)
        lines = [b"[labtether_db_task]\n"]
        for key, value in options.items():
            line = key.encode("ascii") + b"=" + value + b"\n"
            # libpq service lines are raw, limited to 1023 bytes, and right-trimmed.
            if (b"\n" in value or b"\r" in value or value.rstrip() != value
                    or len(line) >= 1023):
                raise ValueError("DATABASE_URL value cannot be represented safely in a service file")
            lines.append(line)
        service = directory / "pg_service.conf"
        private_file(service, b"".join(lines))
        env["PGSERVICEFILE"] = str(service)
        env["PGSERVICE"] = "labtether_db_task"
        yield env


def interrupt(_signum, _frame):
    raise KeyboardInterrupt


def main():
    if len(sys.argv) < 2 or sys.argv[1] not in {"pg_dump", "psql"}:
        return 2
    for signum in (signal.SIGINT, signal.SIGTERM, signal.SIGHUP):
        signal.signal(signum, interrupt)
    try:
        with os.fdopen(3, "rb") as source:
            connection = source.read()
        # Bash adds exactly one newline to the private here-string.
        if connection.endswith(b"\n"):
            connection = connection[:-1]
        with connection_environment(connection, sys.argv[1]) as env:
            process = subprocess.Popen(sys.argv[1:], env=env, close_fds=True)
            try:
                return process.wait()
            finally:
                if process.poll() is None:
                    process.terminate()
                    try:
                        process.wait(timeout=5)
                    except subprocess.TimeoutExpired:
                        process.kill()
                        process.wait()
    except KeyboardInterrupt:
        return 130
    except (OSError, ValueError):
        print("Database client setup failed; check Python 3, libpq and DATABASE_URL (details hidden).", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
