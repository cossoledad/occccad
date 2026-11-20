"""Time two Release Workers on the same accepted Part predecessor."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import sys
import uuid

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]


def overlay(output, name):
    # Separate only the final artifact lookup. Keep identical predecessor bytes and tip inputs.
    source = ROOT / "services/internal/workspace/service.go"
    code = source.read_text()
    marker = "\tkey, err := partGeometryKey(cacheBaseKey, solidFeatures, keyVisualization)"
    if code.count(marker) != 1:
        raise ValueError("Review final artifact lookup before timing this service version")
    salt = str(uuid.uuid4())
    isolate = ('\tif strings.Contains(reqID, "-naming-timing-") && checkpoint.continuation != nil '
               '{ cacheBaseKey += "|shell-probe-' + salt + '" }\n')
    override = output / (name + "-service.go")
    override.write_text(code.replace(marker, isolate + marker))
    (output / (name + ".json")).write_text(json.dumps({"Replace": {str(source): str(override)}}))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--baseline", type=Path, required=True)
    parser.add_argument("--candidate", type=Path, required=True)
    parser.add_argument("--document", required=True, help="accepted growing Part in the development store")
    parser.add_argument("--offset", default=".1875")
    parser.add_argument("--output", type=Path, required=True, help="new directory for the logs and measurements")
    args = parser.parse_args()
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    logs = []
    for name, worker in [("baseline", args.baseline), ("candidate", args.candidate)]:
        overlay(output, name)
        env = {**os.environ, "OCCCCAD_GEOMETRY_WORKER_BIN": str(worker.resolve(strict=True)),
               "OCCCCAD_TEST_NAMING_TIMING_DOCUMENT": args.document, "OCCCCAD_TEST_NAMING_OFFSET_MM": args.offset}
        log = output / (name + ".log")
        with log.open("w") as stream:
            subprocess.run(["go", "test", "-tags=integration", "-count=1", "-overlay", output / (name + ".json"),
                            "./internal/control", "-run", "^TestStoredNamingTipPreviewTimingThroughRouter$", "-v"],
                           cwd=ROOT / "services", env=env, stdout=stream, stderr=subprocess.STDOUT, check=True)
        logs.append(log)
    subprocess.run([sys.executable, HERE / "compare-previews.py", *logs], check=True)


if __name__ == "__main__":
    main()
