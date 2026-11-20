# OCCT 布尔组壳对照

默认 Worker 使用根 Conan 锁文件中的项目 OCCT 包。`mise run cpp:deps --profile release` 从仓库导出配方、核对 recipe revision，然后安装对应包。原生 `geometry/BooleanShell` 测试覆盖索引阈值、接缝方向、带位置的面、多实体并差交、相切、退化边、STEP 切除与取消。`OCCCCAD_TEST_BOOLEAN_REPORT=1` 可额外输出各输入的 BREP 和有序 Modified/Generated/Deleted 历史摘要。

要比较两个已构建的 Worker，使用同一已接纳的增长零件和单独的输出目录。夹具可由 `TestGrowingTopologyPartFeaturesAndNamingThroughRouter` 随 `OCCCCAD_TEST_NAMING_STEPS` 生成，两组使用同一服务代码和检查点策略。每组新起进程，三个高度各执行一个末步。Go 的测试覆盖层只隔离最终制品查询键。测试检查前驱复用、实际执行、预览后的 Head、几何/命名/显示制品逐字节摘要。

```sh
python3 tests/occt-boolean/time-previews.py \
  --baseline "$baseline_worker" \
  --candidate build/cmake/linux-gcc15-x86_64/release/workers/geometry/occccad_geometry_worker \
  --document "$fixture_id" --offset .1875 --output build/performance/part-preview-run
```

计时和工作量记录在各组日志的 `delta_mm` 和 `tip_work` 行。首次导入 STEP 切除另用 `TestDenseSTEPFaceSketchAndFirstCutThroughRouter` 验证材料变化、预览提升、改深度与冷重放。[验证结果](../../docs/validation/part-feature-performance.md#默认内核与末步预览)
