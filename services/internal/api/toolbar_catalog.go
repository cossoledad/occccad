package api

import (
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
	writeJSON(writer, http.StatusOK, map[string]any{"schemaVersion": 1, "toolbars": entries})
}
