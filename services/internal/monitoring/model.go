// Package monitoring defines the transport-neutral monitoring snapshot used by
// the local control plane and presentation adapters such as the TUI.
package monitoring

import "time"

const Schema = "occccad.monitoring.snapshot.v1"

type Snapshot struct {
	Schema      string            `json:"schema"`
	GeneratedAt time.Time         `json:"generatedAt"`
	Host        Host              `json:"host"`
	Processes   []Process         `json:"processes"`
	Geometry    Geometry          `json:"geometry"`
	Business    Business          `json:"business"`
	Parameters  map[string]string `json:"parameters"`
	Warnings    []string          `json:"warnings,omitempty"`
}

type Host struct {
	GoVersion string `json:"goVersion"`
	GOOS      string `json:"goos"`
	GOARCH    string `json:"goarch"`
	CPUs      int    `json:"cpus"`
}

type Process struct {
	ID            string   `json:"id"`
	Kind          string   `json:"kind"`
	PID           int      `json:"pid"`
	Running       bool     `json:"running"`
	CPUPercent    float64  `json:"cpuPercent"`
	ResidentBytes uint64   `json:"residentBytes"`
	VirtualBytes  uint64   `json:"virtualBytes"`
	Threads       int      `json:"threads"`
	UptimeSeconds float64  `json:"uptimeSeconds"`
	Address       string   `json:"address,omitempty"`
	ResidentItems int      `json:"residentItems,omitempty"`
	InFlight      int      `json:"inFlight,omitempty"`
	ResidentKeys  []string `json:"residentKeys,omitempty"`
}

type Geometry struct {
	Workers           int `json:"workers"`
	Minimum           int `json:"minimum"`
	Maximum           int `json:"maximum"`
	ResidentGeometry  int `json:"residentGeometry"`
	InFlight          int `json:"inFlight"`
	CapacityPerWorker int `json:"capacityPerWorker"`
}

type Business struct {
	RealtimeConnections  int            `json:"realtimeConnections"`
	SubscribedDocuments  int            `json:"subscribedDocuments"`
	OpenDocumentSessions int            `json:"openDocumentSessions"`
	Counts               map[string]int `json:"counts"`
	OpenDocuments        []OpenDocument `json:"openDocuments,omitempty"`
}

type OpenDocument struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Sessions int    `json:"sessions"`
}
