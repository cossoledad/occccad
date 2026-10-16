"""One development environment for Invoke, application processes and validation."""
from __future__ import annotations

import os
import json
import hashlib
from pathlib import Path
from urllib.parse import quote, urlsplit


def build_type(explicit: str | None = None) -> str:
    value = (explicit or os.environ.get("OCCCCAD_BUILD_TYPE") or "Release").strip().lower()
    if value not in {"release", "debug"}:
        raise ValueError("build type must be Release or Debug")
    return value.capitalize()


def load(root: Path) -> None:
    explicit = os.environ.get("OCCCCAD_ENV_FILE", "").strip()
    path = Path(explicit).absolute() if explicit else root / ".env"
    if explicit or path.exists():
        os.environ["OCCCCAD_ENV_FILE"] = str(path)
        for raw in path.read_text().splitlines():
            line = raw.strip()
            if not line or line.startswith("#"):
                continue
            if line.startswith("export "):
                line = line.removeprefix("export ").strip()
            key, separator, value = line.partition("=")
            key, value = key.strip(), value.strip()
            if not separator or not key.replace("_", "").isalnum():
                continue
            if len(value) >= 2 and value[0] == value[-1] and value[0] in {"'", '"'}:
                value = value[1:-1]
            os.environ.setdefault(key, value)

    def resolve(path: str, base: Path = root) -> str:
        return os.path.abspath(base / path)

    resource = resolve(os.environ.get("OCCCCAD_TEST_RESOURCES_DIR") or "build/test-resources")
    defaults = {
        "OCCCCAD_TEST_RESOURCES_DIR": resource,
        "OCCCCAD_ASSEMBLY_FIXTURE_DIR": str(Path(resource) / "analytic-fixtures"),
        "OCCCCAD_TEST_IMPORT_BREP": str(Path(resource) / "import-repair.brep"),
        "OCCCCAD_TEST_CURVED_GLB": str(Path(resource) / "curved.glb"),
        "OCCCCAD_TEST_GLB_FIXTURE": str(Path(resource) / "geometry.glb"),
        "OCCCCAD_TEST_ASSEMBLY_REPLAY_OUTPUT": str(Path(resource) / "assembly-replay.3dreplay"),
        "OCCCCAD_TEST_NAMING_FIXTURE": str(Path(resource) / "naming-v2.pb"),
    }
    for key, fallback in defaults.items():
        os.environ[key] = resolve(os.environ.get(key) or fallback)
    for key, fallback in {"OCCCCAD_DATA_DIR": "./data", "OCCCCAD_LOG_DIR": "./logs"}.items():
        os.environ[key] = resolve(os.environ.get(key) or fallback, root / "services")
    if os.environ.get("OCCCCAD_GEOMETRY_WORKER_BIN"):
        os.environ["OCCCCAD_GEOMETRY_WORKER_BIN"] = resolve(os.environ["OCCCCAD_GEOMETRY_WORKER_BIN"])
    dsn = os.environ.get("OCCCCAD_DATABASE_URL", "")
    if dsn.startswith("sqlite:"):
        os.environ["OCCCCAD_DATABASE_URL"] = "sqlite:" + resolve(dsn.removeprefix("sqlite:"))


def command_environment(root: Path, explicit_type: str | None = None) -> dict[str, str]:
    """Resolve the selected build once and pass it to every subprocess.

    Do not store a derived Worker path at module import: a later --build-type
    must select the same artifact for application startup and integration tests.
    """
    selected = build_type(explicit_type)
    worker = os.environ.get("OCCCCAD_GEOMETRY_WORKER_BIN") or str(
        root / "build" / "cmake" / selected.lower() / "workers/geometry/occccad_geometry_worker"
    )
    result = {key: value for key, value in os.environ.items() if key.startswith("OCCCCAD_")}
    result.update(OCCCCAD_BUILD_TYPE=selected, OCCCCAD_GEOMETRY_WORKER_BIN=worker)
    if not result.get("OCCCCAD_DATABASE_URL"):
        value = lambda key, fallback: os.environ.get("OCCCCAD_POSTGRES_" + key) or fallback
        host = value("HOST", "127.0.0.1")
        if ":" in host and not host.startswith("["):
            host = f"[{host}]"
        user = quote(value("USER", "occccad"), safe="")
        password = quote(value("PASSWORD", ""), safe="")
        result["OCCCCAD_DATABASE_URL"] = (
            f"postgres://{user}:{password}@{host}:{value('PORT', '5432')}/"
            f"{quote(value('DB', 'occccad'), safe='')}?search_path=occccad%2Cpublic"
        )
    return result


def environment_identity(environment: dict[str, str]) -> dict:
    dsn = environment.get("OCCCCAD_DATABASE_URL", "")
    if dsn.startswith("sqlite:"):
        database = {"backend": "sqlite", "path": dsn.removeprefix("sqlite:")}
    else:
        parsed = urlsplit(dsn)
        database = {"backend": "postgres", "host": parsed.hostname,
                    "port": parsed.port, "database": parsed.path.lstrip("/")}
    worker = Path(environment["OCCCCAD_GEOMETRY_WORKER_BIN"])
    digest = None
    if worker.is_file():
        with worker.open("rb") as stream:
            digest = hashlib.file_digest(stream, "sha256").hexdigest()
    identity = {"database": database, "buildType": environment["OCCCCAD_BUILD_TYPE"],
                "worker": {"path": str(worker), "sha256": digest},
                "artifactBackend": environment.get("OCCCCAD_ARTIFACT_BACKEND") or "LOCAL",
                "dataDirectory": environment["OCCCCAD_DATA_DIR"],
                "logDirectory": environment["OCCCCAD_LOG_DIR"],
                "listeners": {"app": environment.get("OCCCCAD_APP_LISTEN") or "127.0.0.1:8080",
                              "router": environment.get("OCCCCAD_GEOMETRY_WORKER_ADDRESS") or "127.0.0.1:51001"}}
    if identity["artifactBackend"].upper() == "S3":
        identity["s3"] = {"endpoint": environment.get("OCCCCAD_S3_ENDPOINT"),
                          "bucket": environment.get("OCCCCAD_S3_BUCKET")}
    identity["digest"] = hashlib.sha256(json.dumps(identity, sort_keys=True).encode()).hexdigest()
    return identity


