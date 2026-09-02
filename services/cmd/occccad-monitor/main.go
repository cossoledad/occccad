package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/occccad/occccad/internal/monitoring"
)

type tickMsg time.Time
type snapshotMsg struct {
	snapshot monitoring.Snapshot
	err      error
}

type model struct {
	client             *monitoring.Client
	snapshot           monitoring.Snapshot
	err                error
	width, height, tab int
}

var (
	accent  = lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")).Bold(true)
	dim     = lipgloss.NewStyle().Foreground(lipgloss.Color("#777777"))
	good    = lipgloss.NewStyle().Foreground(lipgloss.Color("#45D483"))
	bad     = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5F87"))
	heading = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#B8A7FF"))
)

func (m model) Init() tea.Cmd { return fetch(m.client) }
func fetch(client *monitoring.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
		defer cancel()
		snapshot, err := client.Fetch(ctx)
		return snapshotMsg{snapshot, err}
	}
}
func tick() tea.Cmd { return tea.Tick(time.Second, func(at time.Time) tea.Msg { return tickMsg(at) }) }

func (m model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab", "right", "l":
			m.tab = (m.tab + 1) % 4
		case "shift+tab", "left", "h":
			m.tab = (m.tab + 3) % 4
		case "1", "2", "3", "4":
			m.tab = int(msg.String()[0] - '1')
		case "r":
			return m, fetch(m.client)
		}
	case snapshotMsg:
		m.err = msg.err
		if msg.err == nil {
			m.snapshot = msg.snapshot
		}
		return m, tick()
	case tickMsg:
		return m, fetch(m.client)
	}
	return m, nil
}

func (m model) View() tea.View {
	tabs := []string{"Overview", "Processes", "Geometry", "Business"}
	for i := range tabs {
		if i == m.tab {
			tabs[i] = accent.Render("[" + tabs[i] + "]")
		} else {
			tabs[i] = dim.Render(tabs[i])
		}
	}
	lines := []string{accent.Render("occccad monitor") + "  " + strings.Join(tabs, "  ")}
	if m.snapshot.Schema == "" {
		lines = append(lines, "", dim.Render("Connecting to the control plane…"))
	} else {
		lines = append(lines, dim.Render(fmt.Sprintf("snapshot %s · %s/%s · %d CPUs", m.snapshot.GeneratedAt.Local().Format("15:04:05"), m.snapshot.Host.GOOS, m.snapshot.Host.GOARCH, m.snapshot.Host.CPUs)), "")
		switch m.tab {
		case 0:
			lines = append(lines, overview(m.snapshot)...)
		case 1:
			lines = append(lines, processView(m.snapshot)...)
		case 2:
			lines = append(lines, geometryView(m.snapshot)...)
		case 3:
			lines = append(lines, businessView(m.snapshot)...)
		}
	}
	if m.err != nil {
		lines = append(lines, "", bad.Render("stale · "+m.err.Error()))
	}
	for _, warning := range m.snapshot.Warnings {
		lines = append(lines, bad.Render("warning · "+warning))
	}
	lines = append(lines, "", dim.Render("←/→ or h/l switch · 1-4 jump · r refresh · q quit"))
	return tea.NewView(strings.Join(lines, "\n"))
}

func overview(s monitoring.Snapshot) []string {
	running, rss, cpu := 0, uint64(0), 0.0
	for _, p := range s.Processes {
		if p.Running {
			running++
		}
		rss += p.ResidentBytes
		cpu += p.CPUPercent
	}
	return []string{heading.Render("System"), fmt.Sprintf("  Processes  %d/%d running", running, len(s.Processes)),
		fmt.Sprintf("  CPU        %6.1f%%", cpu), fmt.Sprintf("  RSS        %s", bytes(rss)), "",
		heading.Render("Workload"), fmt.Sprintf("  Documents  %d persistent · %d open sessions", s.Business.Counts["documents"], s.Business.OpenDocumentSessions),
		fmt.Sprintf("  Geometry   %d resident · %d in flight · %d workers", s.Geometry.ResidentGeometry, s.Geometry.InFlight, s.Geometry.Workers),
		fmt.Sprintf("  Realtime   %d connections · %d subscribed documents", s.Business.RealtimeConnections, s.Business.SubscribedDocuments)}
}

func processView(s monitoring.Snapshot) []string {
	lines := []string{heading.Render(fmt.Sprintf("%-18s %-6s %-9s %8s %10s %8s %8s", "PROCESS", "PID", "STATE", "CPU", "RSS", "THREADS", "UPTIME"))}
	for _, p := range s.Processes {
		state := bad.Render("down")
		if p.Running {
			state = good.Render("running")
		}
		lines = append(lines, fmt.Sprintf("%-18s %-6d %-18s %7.1f%% %10s %8d %7s", p.ID, p.PID, state, p.CPUPercent, bytes(p.ResidentBytes), p.Threads, duration(p.UptimeSeconds)))
	}
	return lines
}

func geometryView(s monitoring.Snapshot) []string {
	lines := []string{heading.Render("Geometry pool"), fmt.Sprintf("  Workers             %d  (min %d / max %d)", s.Geometry.Workers, s.Geometry.Minimum, s.Geometry.Maximum),
		fmt.Sprintf("  Resident geometry   %d", s.Geometry.ResidentGeometry), fmt.Sprintf("  In flight           %d", s.Geometry.InFlight), fmt.Sprintf("  Soft capacity       %d per worker", s.Geometry.CapacityPerWorker), ""}
	for _, p := range s.Processes {
		if p.Kind == "geometry" {
			lines = append(lines, fmt.Sprintf("  %-16s %s · resident %d · in-flight %d · %s RSS", p.ID, p.Address, p.ResidentItems, p.InFlight, bytes(p.ResidentBytes)))
			for _, key := range p.ResidentKeys {
				lines = append(lines, dim.Render("    ↳ "+key))
			}
		}
	}
	return lines
}

func businessView(s monitoring.Snapshot) []string {
	lines := []string{heading.Render("Live business state"), fmt.Sprintf("  Realtime connections     %d", s.Business.RealtimeConnections), fmt.Sprintf("  Subscribed documents     %d", s.Business.SubscribedDocuments), fmt.Sprintf("  Open document sessions   %d", s.Business.OpenDocumentSessions), "", heading.Render("Persistent state")}
	keys := make([]string, 0, len(s.Business.Counts))
	for key := range s.Business.Counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("  %-24s %d", key, s.Business.Counts[key]))
	}
	if len(s.Business.OpenDocuments) > 0 {
		lines = append(lines, "", heading.Render("Open documents"))
		for _, document := range s.Business.OpenDocuments {
			lines = append(lines, fmt.Sprintf("  %-24s %-8s %d session(s) · %s", document.Name, document.Type, document.Sessions, document.ID))
		}
	}
	lines = append(lines, "", heading.Render("Runtime parameters"))
	keys = keys[:0]
	for key := range s.Parameters {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("  %-24s %s", key, s.Parameters[key]))
	}
	return lines
}

func bytes(value uint64) string {
	units := []string{"B", "KiB", "MiB", "GiB"}
	amount := float64(value)
	unit := 0
	for amount >= 1024 && unit < len(units)-1 {
		amount /= 1024
		unit++
	}
	return fmt.Sprintf("%.1f %s", amount, units[unit])
}
func duration(seconds float64) string {
	return (time.Duration(seconds) * time.Second).Round(time.Second).String()
}

func main() {
	address := os.Getenv("OCCCCAD_CONTROL_URL")
	if address == "" {
		address = "http://127.0.0.1:19090"
	}
	program := tea.NewProgram(model{client: monitoring.NewClient(address)})
	if _, err := program.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
