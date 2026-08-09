# occccad

> **Cloud-native distributed CAD platform** — built on Open CASCADE Technology.

occccad is not "a web wrapper around a desktop CAD kernel." It is a
ground-up cloud-native CAD infrastructure with:

- **Content-addressable geometry** (SHA-256 GeometryId)
- **Document/Version model** independent of worker processes
- **Coarse-grained geometry RPC** (move computation to data)
- **Genuine distributed workers** with load/unload/recovery
- **B-Rep + Topology** semantics reaching the browser
- **Command/Transaction framework** with full Undo/Redo audit trail

---

## Architecture (V0.1)

```
Browser (TypeScript/React/Three.js)
    |
Gateway (Go)
    |
+---+---+---+---+
| Doc | Part | Product | Command |
+---+---+---+---+
    |
Geometry Router / Scheduler
    |
+-----+-----+-----+
| GW1  | GW2  | GW3 |   C++ / OCCT Workers
+-----+-----+-----+
    |
PostgreSQL | Redis | MinIO
```

See [`docs/`](docs/) for the full architecture specification.

---

## Prerequisites

| Component | Required |
|-----------|----------|
| GCC 14+ | C++23 compiler |
| CMake >= 3.30 | Build system |
| Ninja | Fast build |
| Conan 2.x | C++ package manager |
| Python 3.12+ | CLI tooling (invoke) |
| Go 1.26+ | Control plane services |
| Node.js 24 LTS | Frontend |
| pnpm 9+ | Frontend package manager |

---

## Quick Start

```bash
# 1. Install tool dependencies
invoke bootstrap

# 2. Copy and edit service addresses
cp .env.example .env

# 3. Configure & build C++ components
invoke configure debug
invoke build

# 4. Run tests
invoke test

# 5. Run the geometry worker smoke test
invoke run.geometry

# 6. Check everything
invoke info
```

---

## Commands

```
invoke bootstrap         Install Python deps, set up Conan profiles
invoke info              Show toolchain versions and paths

invoke configure [debug|release]   Conan install + CMake configure
invoke build [--target=...]        Build C++ targets
invoke test [--filter=...]         Run C++ tests

invoke run.geometry                Build + run geometry worker
invoke clean                       Remove build artifacts
```

---

## Project Layout

```
occccad/
├── CMakeLists.txt                  # Root CMake
├── CMakePresets.json               # CMake presets (dev-debug, dev-release, ci-debug)
├── conanfile.py                    # C++ dependency graph
├── tasks.py                        # Invoke CLI (configure, build, test, etc.)
│
├── build-support/conan/
│   ├── profiles/                   # linux-gcc14-debug, linux-gcc14-release, linux-clang-debug
│   └── locks/                      # Conan lockfiles (reproducible builds)
│
├── cmake/                          # CMake modules (warnings, sanitizers)
├── proto/                          # Protobuf definitions
│
├── kernel/
│   ├── api/                        # Public C++ API (ICadKernel, GeometryId, TopologyRef)
│   └── occt/                       # OCCT adapter (isolated from business logic)
│
├── workers/geometry/               # Geometry Worker (C++ gRPC server)
├── services/                       # Go control-plane services
├── web/                            # TypeScript/React frontend
├── tests/                          # C++ tests
└── docs/                           # Architecture & toolchain specs
```

---

## External Services

This repository does **not** manage infrastructure. You need running instances of:

| Service | Default Address | Purpose |
|---------|----------------|---------|
| PostgreSQL | `127.0.0.1:5432` | Document, version, command storage |
| Redis | `127.0.0.1:6379` | Worker registry, job leases, cache |
| S3 (MinIO) | `127.0.0.1:9000` | B-Rep, GLB, mesh artifacts |

Configure addresses in `.env` (copy from `.env.example`).

---

## C++ Dependencies

All C++ dependencies are managed by Conan 2:

| Library | Purpose |
|---------|---------|
| OCCT 8.0.0 | CAD kernel (B-Rep, STEP, tessellation) |
| fmt | String formatting |
| spdlog | Structured logging |
| gRPC + Protobuf | RPC framework |
| Eigen | Linear algebra |
| Ceres Solver | Constraint solving |
| GoogleTest | Unit testing |
| Google Benchmark | Performance benchmarks |

---

## License

TBD

---

> **Document != Geometry. Document != Worker.**
> — occccad Architecture Principle #1
