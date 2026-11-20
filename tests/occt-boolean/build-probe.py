"""Build an isolated OCCT 7.9.1 Boolean experiment with existing Release Ninja builds."""

import argparse
import filecmp
import json
from pathlib import Path
import shlex
import shutil
import subprocess
import tempfile
import uuid


HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
PATCH = ROOT / "build-support/conan/patches/opencascade/7.9.1/shell-face-edge-lookup.patch"
WORKER = "workers/geometry/occccad_geometry_worker"


def ninja_command(build, target):
    lines = subprocess.check_output(["ninja", "-t", "commands", target], cwd=build, text=True)
    return lines.splitlines()[-1]


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
    parser.add_argument("--occt-build", type=Path, required=True, help="OCCT Release Ninja build")
    parser.add_argument("--worker-build", type=Path, required=True, help="occccad Release Ninja build")
    parser.add_argument("--output", type=Path, required=True, help="new experiment directory")
    args = parser.parse_args()
    occt, worker, output = (p.resolve() for p in (args.occt_build, args.worker_build, args.output))
    if '#define OCC_VERSION_COMPLETE "7.9.1"' not in (occt / "include/Standard_Version.hxx").read_text():
        parser.error("expected OCCT 7.9.1")
    link = shlex.split(ninja_command(worker, WORKER).split(" && ")[1])
    archives = [Path(arg) for arg in link if arg.endswith("/libTKBO.a")]
    if len(archives) != 1 or not filecmp.cmp(occt / "lin64/gcc/lib/libTKBO.a", archives[0], shallow=False):
        parser.error("expected the static TKBO archive from the supplied Linux GCC OCCT build")
    output.mkdir(parents=True, exist_ok=False)
    with tempfile.TemporaryDirectory(dir=output) as tmp:
        stage = Path(tmp)
        objects = []
        for package, name in [("BOPAlgo", "BOPAlgo_ShellSplitter"), ("BOPTools", "BOPTools_AlgoTools")]:
            target = f"src/TKBO/CMakeFiles/TKBO.dir/__/{package}/{name}.cxx.o"
            compile_args = shlex.split(ninja_command(occt, target))
            source = Path(compile_args[compile_args.index("-c") + 1])
            dest = stage / "src" / package / source.name
            dest.parent.mkdir(parents=True)
            shutil.copyfile(source, dest)
            if package == "BOPTools":
                shutil.copyfile(source.with_name("FILES"), dest.with_name("FILES"))
            objects.append((compile_args, dest, stage / (name + ".o")))
        subprocess.run(["patch", "--batch", "--fuzz=0", "-p1", "-i", PATCH], cwd=stage, check=True)

        def compile_one(args, source, obj):
            args = args.copy()
            for flag, value in [("-c", source), ("-o", obj), ("-MT", obj), ("-MF", str(obj) + ".d")]:
                args[args.index(flag) + 1] = str(value)
            args.insert(1, "-I" + str(stage / "src/BOPTools"))
            subprocess.run(args, cwd=occt, check=True)

        for args, source, obj in objects:
            compile_one(args, source, obj)
        oracle = stage / "oracle.o"
        compile_one(objects[0][0], HERE / "face-edge-oracle.cpp", oracle)
        link = [arg for arg in link if not arg.startswith("-Wl,--dependency-file=")]
        main_object = WORKER.rsplit("/", 1)[0] + "/CMakeFiles/occccad_geometry_worker.dir/src/main.cpp.o"
        main_index = link.index(main_object)
        link[link.index("-o") + 1] = str(output / "lookup-worker")
        link[main_index:main_index] = [str(obj) for _, _, obj in objects]
        subprocess.run(link, cwd=worker, check=True)
        link[link.index(main_object)] = str(oracle)
        link[link.index("-o") + 1] = str(stage / "oracle")
        subprocess.run(link, cwd=worker, check=True)
        subprocess.run([stage / "oracle"], check=True)
    for name in ["base", "candidate"]:
        overlay(output, name)


if __name__ == "__main__":
    main()
