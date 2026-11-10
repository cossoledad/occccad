package api

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/visualdelta"
)

// Display encodings share a bounded CPU/memory slot. Saturation uses the full
// immutable representation; no geometry job or model write is introduced.
var visualDeltaSlot = make(chan struct{}, 1)

func acceptsVisualDelta(accept string) bool {
	for _, item := range strings.Split(accept, ",") {
		kind, parameters, err := mime.ParseMediaType(strings.TrimSpace(item))
		if err != nil || kind != visualdelta.ContentType {
			continue
		}
		if value, ok := parameters["q"]; ok {
			q, err := strconv.ParseFloat(value, 64)
			if err != nil || !(q > 0 && q <= 1) {
				continue
			}
		}
		return true
	}
	return false
}

func (s *Server) serveVisualDelta(w http.ResponseWriter, r *http.Request, target artifact.Object, reader io.Reader) (io.Reader, bool) {
	basePath := r.URL.Query().Get("base")
	if basePath != "" {
		w.Header().Add("Vary", "Accept")
	}
	if basePath == "" || !acceptsVisualDelta(r.Header.Get("Accept")) || target.Kind != artifact.KindGLB || target.Size < 4096 || target.Size > visualdelta.MaxBytes {
		return reader, false
	}
	if len(basePath) > 4096 {
		writeError(w, http.StatusBadRequest, "invalid visual base")
		return nil, true
	}
	u, e := url.Parse(basePath)
	if e != nil || u.IsAbs() || u.Host != "" || u.Fragment != "" {
		writeError(w, http.StatusBadRequest, "invalid visual base")
		return nil, true
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 5 || parts[0] != "api" || parts[1] != "documents" || parts[2] != r.PathValue("documentID") || parts[3] != "representations" || parts[4] == "" {
		writeError(w, http.StatusBadRequest, "invalid visual base scope")
		return nil, true
	}
	baseRequest := r.Clone(r.Context())
	baseRequest.URL = u
	baseRequest.SetPathValue("objectID", parts[4])
	baseRequest.SetPathValue("documentID", parts[2])
	if !s.authorizeRepresentation(w, baseRequest) {
		return nil, true
	}
	if parts[4] == target.ID {
		return reader, false
	}
	select {
	case visualDeltaSlot <- struct{}{}:
		defer func() { <-visualDeltaSlot }()
	default:
		return reader, false
	}
	base, baseReader, e := s.artifacts.Open(r.Context(), parts[4])
	if e != nil {
		writeError(w, http.StatusNotFound, "visual base unavailable")
		return nil, true
	}
	defer baseReader.Close()
	if base.Kind != artifact.KindGLB || base.Size > visualdelta.MaxBytes || base.Size < 4096 {
		return reader, false
	}
	baseBytes, e := io.ReadAll(io.LimitReader(baseReader, visualdelta.MaxBytes+1))
	if e != nil || int64(len(baseBytes)) != base.Size {
		writeError(w, http.StatusInternalServerError, "visual base read failed")
		return nil, true
	}
	targetBytes, e := io.ReadAll(io.LimitReader(reader, visualdelta.MaxBytes+1))
	if e != nil || int64(len(targetBytes)) != target.Size {
		writeError(w, http.StatusInternalServerError, "visual target read failed")
		return nil, true
	}
	patch, e := visualdelta.Encode(r.Context(), baseBytes, targetBytes, base.SHA256, target.SHA256)
	if r.Context().Err() != nil {
		return nil, true
	}
	if e != nil || len(patch)*10 >= len(targetBytes)*9 {
		return bytes.NewReader(targetBytes), false
	}
	w.Header().Set("Content-Type", visualdelta.ContentType)
	digest := sha256.Sum256(patch)
	etag := fmt.Sprintf(`"%x"`, digest)
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return nil, true
	}
	w.Header().Set("Content-Length", fmt.Sprint(len(patch)))
	w.Header().Set("X-OCCCCAD-Visual-Target-Size", fmt.Sprint(target.Size))
	w.Header().Set("Content-Disposition", `inline; filename="geometry.mesh.delta"`)
	_, _ = w.Write(patch)
	return nil, true
}
