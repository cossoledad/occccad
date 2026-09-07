# occccad-3dreplay

Replay a three-dimensional assembly solve using only a `.3dreplay` file and a running Geometry Worker or Router:

```sh
# From services/
go run ./cmd/occccad-3dreplay -worker 127.0.0.1:51001 \
  -out /tmp/replayed.3dreplay ../tests/assembly-corpus/face4-face6.3dreplay
```

The output records the new solver result, including non-convergence; compare it with the input file's observed result. A numerical failure remains useful replay output, while malformed files and unsupported schemas fail the command. The RPC deadline is 30 seconds.

`occccad.3dreplay.v1` is JSON with:

- `schema` and `units` (mm/rad, body-local geometry, quaternion xyzw);
- `request`: the typed `SolveAssemblyRequest` in Proto JSON — body nominal poses and optional guesses, mathematical Point/Axis/Plane/Cylinder descriptors, Fix/Rigid/other constraints, branch/solve intent, affected scope, residual scales, and the effective solver profile;
- `result`: solved/candidate poses, status, residuals, ranks, component and motion-preference evidence, diagnostics, branch output and solver build;
- `transportError` when the RPC did not return a numerical result.

Proto JSON omits default-valued scalars (including zero coordinates), and writes 64-bit integers as decimal strings. The effective profile freezes server defaults when a numerical response exists. Full null-space matrices, detailed freedom bases, B-Rep, meshes, document history and browser state are excluded. Geometry IDs only match references inside this file; replay does not resolve topology externally.

The Product preview panel and Product Debug toolbar download the current request's file. The service exposes the most recent 50 local record IDs and downloads by ID; document read permission is required. Files live under `OCCCCAD_LOG_DIR/debug/assembly-replays`, are retained for at most seven days, and never enter PostgreSQL or model revisions. Records include preview and failed solves and survive API restarts on the same host. A failure before mathematical geometry resolution creates no replay, and a missing request returns 404. Existing older solves cannot be recovered retroactively; the supplied regression was reconstructed from its unchanged database revision and exact OCCT descriptors before the fix.

Replay establishes numerical reproducibility and comparison within the recorded branch/build context; it does not promise identical solutions across future solver builds or replace the planned full Product SolveManifest. Local debug retention is deliberately bounded and is not a durable audit facility.
