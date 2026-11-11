package exchange

// Warning describes source content that was not created in the application.
// It never turns a successful file read into a failed import.
type Warning struct {
	Code         string `json:"code"`
	Message      string `json:"message"`
	DefinitionID string `json:"definitionId,omitempty"`
	Detail       string `json:"detail,omitempty"`
	Count        uint64 `json:"count,omitempty"`
}

type Report struct {
	SchemaVersion int                `json:"schemaVersion"`
	SourceSHA256  string             `json:"sourceSha256,omitempty"`
	SourceSchema  string             `json:"sourceSchema,omitempty"`
	KernelVersion string             `json:"kernelVersion,omitempty"`
	Warnings      []Warning          `json:"warnings,omitempty"`
	Definitions   int                `json:"definitions"`
	EmptyParts    int                `json:"emptyParts"`
	TimingsMS     map[string]float64 `json:"timingsMs,omitempty"`
}

func WarningMessage(code, detail string) string {
	switch code {
	case "STEP_NO_TRANSFERABLE_GEOMETRY":
		return "转换器未生成可创建的几何；源文件已保留。"
	case "STEP_EXTERNAL_REFERENCES_SKIPPED":
		return "未加载源文件引用的外部文件；已将文件内嵌几何导入为一个 Part，未保留外部装配层级和样式。"
	case "STEP_VISIBILITY_LIMITATION":
		return "已包含隐藏对象。当前 STEP 转换器不能完整保留对象的隐藏状态；在其他 CAD 中打开后，这些对象可能显示。"
	case "STEP_TESSELLATED_GEOMETRY_SKIPPED":
		return "已跳过当前不支持的离散网格几何；源文件已保留，未将网格转换为精确实体。"
	case "STEP_LAYERS_SKIPPED":
		return "源文件的图层分组尚不支持，未创建对应图层数据。"
	case "STEP_PMI_SKIPPED":
		return "文件中的 PMI 标注暂不支持，未创建对应数据；可用的模型内容已保留。"
	case "STEP_UNSUPPORTED_ENTITIES":
		return "文件包含当前转换器不支持的元素，这些元素未创建对应数据。"
	case "EXCHANGE_EMPTY_PART":
		return "该零件没有可创建的几何，已创建空 Part 文档。"
	case "EXCHANGE_UNSUPPORTED_GEOMETRY":
		return "部分几何尚不能创建为本软件的数据，已跳过并保留原始文件。"
	case "EXCHANGE_FACE_APPEARANCE_SKIPPED_AFTER_REPAIR":
		return "几何修复后无法确认部分分面外观的对应关系，已保留几何并跳过这些分面外观。"
	default:
		return detail
	}
}

// Realtime events carry category summaries; definition-level details remain in
// the persisted report for inspection after reconnecting.
func (r *Report) NotificationWarnings() []Warning {
	var out []Warning
	indices := map[string]int{}
	for _, warning := range r.Warnings {
		key := warning.Code + "\x00" + warning.Message
		count := warning.Count
		if count == 0 {
			count = 1
		}
		if i, ok := indices[key]; ok {
			out[i].Count += count
			continue
		}
		indices[key] = len(out)
		warning.DefinitionID = ""
		warning.Count = count
		out = append(out, warning)
	}
	return out
}
