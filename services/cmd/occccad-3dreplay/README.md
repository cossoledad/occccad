# occccad-3dreplay

Replay a three-dimensional assembly solve using only a `.3dreplay` file and a running Geometry Worker or Router:

```sh
# From services/
go run ./cmd/occccad-3dreplay -worker 127.0.0.1:51001 \
  -out /tmp/replayed.3dreplay ../tests/test.data/face4-face6.3dreplay
```

The output records the new solver result, including non-convergence; compare it with the input file's observed result. A numerical failure remains useful replay output, while malformed files and unsupported schemas fail the command. The RPC deadline is 30 seconds.

`occccad.3dreplay.v1` is JSON with:

- `schema` and `units` (mm/rad, body-local geometry, quaternion xyzw);
- `request`: the typed `SolveAssemblyRequest` in Proto JSON — body nominal poses and optional guesses, mathematical Point/Axis/Plane/Cylinder descriptors, Fix/Rigid/other constraints, branch/solve intent, affected scope, residual scales, and the effective solver profile;
- `result`: solved/candidate poses, status, aggregate residuals, component rank and motion-preference evidence, a summary diagnostic, branch output and solver build;
- `transportError` when the RPC did not return a numerical result.

Proto JSON omits default-valued scalars (including zero coordinates), and writes 64-bit integers as decimal strings. The effective profile freezes server defaults when a numerical response exists. Full null-space matrices, detailed freedom bases, B-Rep, meshes, document history and browser state are excluded. Geometry IDs only match references inside this file; replay does not resolve topology externally. Oversized runtime geometry IDs containing business reference recipes are replaced by deterministic private SHA-256 aliases consistently in descriptors and all constraint references. Live domain identities and RPC inputs are unchanged. JSON is compact; per-equation residuals, constraint rank tables and duplicate detailed diagnostics are omitted.

The DEBUG toolbar registers separate Part and assembly diagnostic commands. Assembly export reads the current Product revision through `GET /api/documents/{id}/assembly-diagnostic`, not the last preview. The file adds `snapshot` (`occccad.assembly-diagnostic.v1`): all definitions and evaluation/suppression/measurement states, the Product model, exact production-resolved geometry and solver bodies, the frozen solver/group manifest, and recursively referenced Part/Product revision models. Unresolved definitions remain inspectable; unavailable source revisions are marked. Download requires document read permission. It performs no solve or model/history/warm-start write.

Complete diagnostic replay runs the production frozen-manifest and staged group solver without a database or B-Rep. It solves the accepted Verified subset by default. To retry resolvable NotUpdated definitions without changing their saved state:

```sh
go run ./cmd/occccad-3dreplay -worker 127.0.0.1:51001 \
  -include-unverified -out /tmp/retried.3dreplay assembly.3dreplay
```

The complete snapshot survives replay; `assemblyResult` includes domain-level staged group and preference evidence. `result` holds the last numerical RPC result when available. Suppressed and Broken definitions are not activated by this flag. Geometry and manifest input are validated; corrupt digests are rejected. Both complete diagnostics and the original standalone numerical format are supported.

Individual numerical solve records and immutable CAD_DIAGNOSTIC failure snapshots remain bounded local evidence under `OCCCCAD_LOG_DIR/debug`. They preserve failed trials independently from the accepted subset and never authorize failed pose promotion. The inspector and constraint panel expose a copyable diagnostic ID; the toolbar exports the complete current assembly. No history, mesh or B-Rep bytes are included, and reconstruction of unresolved topology requires its original artifacts. Numerical reproducibility does not promise identical solutions across future solver builds.

Assembly integration fixtures use PostgreSQL through the shared `internal/testsupport.OpenPostgres` entry. Set `OCCCCAD_TEST_DATABASE_URL` to a dedicated `occccad_*_test` database (for example `occccad_assembly_contract_test`) on the configured PostgreSQL server, and set `OCCCCAD_TEST_GEOMETRY_WORKER` to the matching binary. The helper validates the actual database identity and applies/checks the current migration chain. It never resets data, accepts application databases, or falls back to SQLite. Fixtures allocate fresh document IDs and their own temporary ArtifactStore. An old migration chain fails explicitly; prepare a current baseline test database instead of changing migration checksums. See [development environment](../../../docs/development-environment.md).
