package api

import (
	"github.com/occccad/occccad/internal/assemblycontract"
	"net/http"
)

type toolbarCatalogItem struct {
	CommandID  string `json:"commandId"`
	Name       string `json:"name"`
	HelpText   string `json:"helpText"`
	IconKey    string `json:"iconKey"`
	GroupKey   string `json:"groupKey"`
	SortOrder  int    `json:"sortOrder"`
	Repeatable bool   `json:"repeatable"`
}

type toolbarCatalogEntry struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Workbench   string               `json:"workbench"`
	Position    string               `json:"position"`
	Orientation string               `json:"orientation"`
	StyleKey    string               `json:"styleKey"`
	SortOrder   int                  `json:"sortOrder"`
	Items       []toolbarCatalogItem `json:"items"`
}

func (server *Server) toolbarCatalog(writer http.ResponseWriter, request *http.Request) {
	rows, err := server.database.Query(request.Context(), `SELECT t.id,t.name,t.workbench,t.position,t.orientation,t.style_key,t.sort_order,
		i.command_id,i.name,i.help_text,i.icon_key,i.group_key,i.sort_order,i.repeatable
		FROM occccad.ui_toolbars t JOIN occccad.ui_toolbar_items i ON i.toolbar_id=t.id
		WHERE t.enabled ORDER BY t.sort_order,i.sort_order`)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	entries := []toolbarCatalogEntry{}
	index := map[string]int{}
	for rows.Next() {
		var toolbar toolbarCatalogEntry
		var item toolbarCatalogItem
		if err := rows.Scan(&toolbar.ID, &toolbar.Name, &toolbar.Workbench, &toolbar.Position, &toolbar.Orientation,
			&toolbar.StyleKey, &toolbar.SortOrder, &item.CommandID, &item.Name, &item.HelpText, &item.IconKey,
			&item.GroupKey, &item.SortOrder, &item.Repeatable); err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		position, ok := index[toolbar.ID]
		if !ok {
			position = len(entries)
			index[toolbar.ID] = position
			toolbar.Items = []toolbarCatalogItem{}
			entries = append(entries, toolbar)
		}
		entries[position].Items = append(entries[position].Items, item)
	}
	if err := rows.Err(); err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"schemaVersion": 1, "toolbars": canonicalAssemblyToolbars(entries)})
}

// Legacy database presentation rows are projected onto the single public model;
// their stored IDs are not business identities and need no data-reset migration.
func canonicalAssemblyToolbars(entries []toolbarCatalogEntry) []toolbarCatalogEntry {
	families := map[string]bool{}
	for _, family := range assemblycontract.Read().Families {
		families[family] = true
	}
	for i := range entries {
		if entries[i].Workbench != "ASSEMBLY_DESIGN" {
			continue
		}
		constraintBar := false
		contactFound := false
		for j := range entries[i].Items {
			item := &entries[i].Items[j]
			switch item.CommandID {
			case "assembly.rigid":
				if families["FixTogether"] {
					item.CommandID = "assembly.fix_together"
					item.Name = "固联组"
					item.HelpText = "管理两个或多个组件及已有固联组；组内先解、组外整体求解。"
				}
			case "assembly.distance":
				item.Name = "偏移"
				item.HelpText = "精确 Point/Line/Plane 偏移；含平面时使用显式法向符号。"
			case "assembly.parallel", "assembly.perpendicular":
				item.HelpText = "Angle 关系族快捷入口；不是指定轴投影角。"
			case "assembly.contact":
				contactFound = true
			}
			if item.CommandID == "assembly.coincident" {
				constraintBar = true
			}
		}
		if constraintBar && !contactFound && families["Contact"] {
			entries[i].Items = append(entries[i].Items, toolbarCatalogItem{CommandID: "assembly.contact", Name: "接触", HelpText: "精确解析面、线、点、环接触；材料侧与分支显式。", IconKey: "tangent", GroupKey: "primary", SortOrder: 65})
		}
	}
	return entries
}
