"""Compare the three executed tip previews emitted by the real Router timing test."""

import argparse
import json
from pathlib import Path
import re


def read_samples(path):
    log = path.read_text()
    if "--- PASS: TestStoredNamingTipPreviewTimingThroughRouter" not in log:
        raise ValueError(f"missing completed timing test in {path}")
    executions = re.findall(r"delta_mm=(\S+) exact_ms=([.\d]+) boolean_ms=([.\d]+)", log)
    previews = re.findall(r"delta_mm=(\S+) elapsed_ms=([.\d]+).*? prefix=(\d+) brep=(\w+) naming=(\w+) visual=(\w+)", log)
    if len(executions) != 3 or len(previews) != 3:
        raise ValueError(f"expected three executed previews in {path}")
    samples = []
    for (delta, exact, boolean), (offset, elapsed, prefix, *digests) in zip(executions, previews):
        if delta != offset or any(not re.fullmatch("[0-9a-f]{64}", digest) for digest in digests):
            raise ValueError(f"incomplete sample at {delta} mm in {path}")
        samples.append((delta, int(prefix), digests, dict(boolean_ms=float(boolean),
                                                         exact_ms=float(exact), preview_ms=float(elapsed))))
    return samples


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("baseline", type=Path)
    parser.add_argument("candidate", type=Path)
    args = parser.parse_args()
    results = []
    for base, candidate in zip(read_samples(args.baseline), read_samples(args.candidate)):
        if base[:3] != candidate[:3]:
            raise ValueError(f"parameter, frozen prefix or artifacts differ at {base[0]} mm")
        results.append(dict(delta_mm=float(base[0]), prefix=base[1], baseline=base[3], candidate=candidate[3],
                            brep=base[2][0], naming=base[2][1], visual=base[2][2]))
    print(json.dumps(results, indent=2))


if __name__ == "__main__":
    main()
