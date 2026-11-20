# OCCT 布尔组壳对照实验

实验使用项目已构建的 Linux GCC Release 静态 Worker、与其完全匹配的 OCCT 7.9.1 Ninja 构建目录、Python 3 和 GNU patch。OCCT 构建目录须保留原始源码和生成头。每次使用新输出目录。构建脚本在临时目录应用补丁，复用上游两个编译单元的编译参数，在实验 Worker 中链接替换这两个对象，并运行面边查找的遍历对照。默认构建通过根 conanfile.py 管理。

```sh
worker_build="$PWD/build/cmake/linux-gcc15-x86_64/release"
out="$PWD/build/performance/shell-lookup-run"
python3 tests/occt-boolean/build-probe.py --occt-build "$occt_build" \
  --worker-build "$worker_build" --output "$out"
```

在项目开发环境运行。将 `occt_build` 指向上述 OCCT 的 Release 构建目录，将 `OCCCCAD_TEST_NAMING_TIMING_DOCUMENT` 设为已有增长夹具的文档 ID。可由 `TestGrowingTopologyPartFeaturesAndNamingThroughRouter` 配合 `OCCCCAD_TEST_NAMING_STEPS` 构造夹具。两组共用同一个文档和起始高度增量，每组新起 Worker。Go overlay 仅对 timing 用例的最终制品查询键加盐，实际几何输入、前驱和服务源码相同。当前用例校验确实执行一个末步、产生一个新的命名块，预览后 Head 相同。

```sh
export OCCCCAD_TEST_NAMING_TIMING_DOCUMENT OCCCCAD_TEST_NAMING_OFFSET_MM
(
  set -e
  cd services
  for mode in base candidate; do
    worker="$worker_build/workers/geometry/occccad_geometry_worker"
    if [ "$mode" = candidate ]; then worker="$out/lookup-worker"; fi
    OCCCCAD_GEOMETRY_WORKER_BIN="$worker" go test -tags=integration -count=1 \
      -overlay "$out/$mode.json" ./internal/control \
      -run '^TestStoredNamingTipPreviewTimingThroughRouter$' -v > "$out/$mode.log" 2>&1
  done
)
python3 tests/occt-boolean/compare-previews.py "$out/base.log" "$out/candidate.log"
```

对照脚本检查三组参数、前驱步数和 BREP/NAMING/VISUAL 的 SHA-256。密集 STEP 另用 `TestDenseSTEPFaceSketchAndFirstCutThroughRouter`，分别设置上述两个 Worker 路径运行。它每次独立生成 STEP 并导入，按精确材料变化及每次运行自己的冷重放验收。[结果和接入条件](../../docs/validation/part-feature-performance.md#布尔组壳的重复遍历)
