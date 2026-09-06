package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/occccad/occccad/internal/access"
)

func (server *Server) listAssemblyReplays(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.requireDocument(w, r, access.RoleViewer); !ok {
		return
	}
	rows, err := server.database.Query(r.Context(), `SELECT jsonb_build_object('id',id,'requestId',request_id,'createdAt',created_at,'status',COALESCE(replay->'result'->>'status','TRANSPORT_ERROR')) FROM occccad.assembly_replays WHERE document_id=$1 ORDER BY created_at DESC,id LIMIT 50`, r.PathValue("documentID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	items := []json.RawMessage{}
	for rows.Next() {
		var item json.RawMessage
		if err = rows.Scan(&item); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (server *Server) downloadAssemblyReplay(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.requireDocument(w, r, access.RoleViewer); !ok {
		return
	}
	var data []byte
	var id string
	err := server.database.QueryRow(r.Context(), `SELECT id,replay FROM occccad.assembly_replays WHERE document_id=$1 AND ($2='latest' OR id=$2) AND ($3='' OR request_id=$3) ORDER BY created_at DESC,id LIMIT 1`, r.PathValue("documentID"), r.PathValue("replayID"), r.URL.Query().Get("requestId")).Scan(&id, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "该次请求尚无三维求解记录；几何解析前失败不会产生 3dreplay")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var pretty json.RawMessage = data
	encoded, err := json.MarshalIndent(pretty, "", "  ")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/vnd.occccad.3dreplay+json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.3dreplay"`, id))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(encoded)
}
