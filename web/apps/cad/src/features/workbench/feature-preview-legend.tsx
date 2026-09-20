import { featurePreviewColors, type FeaturePreviewOperation } from "../../cad/rendering/feature-preview";
import { palette } from "../../design/visual-tokens";
const labels = { NEW_BODY: "新建实体", ADD: "融合结果", REMOVE: "切除结果", INTERSECT: "交集结果" };
export function FeaturePreviewLegend({ operation }: { operation: FeaturePreviewOperation }) {
  return <div className="feature-preview-legend" aria-label="预览图例">
    <span><i style={{ background: featurePreviewColors[operation] }} />{labels[operation]}</span>
    <span><i className="reference" style={{ borderColor: palette.previewReference }} />原始实体参考</span>
  </div>;
}
