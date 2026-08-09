package control

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

type GeometryPoolConfig struct {
	WorkerBinary     string
	WorkerHost       string
	FirstWorkerPort  int
	MinimumWorkers   int
	MaximumWorkers   int
	GeometryCapacity int
	IdleTimeout      time.Duration
}

type workerInstance struct {
	id         string
	address    string
	process    *ManagedProcess
	connection *grpc.ClientConn
	client     workerv1.GeometryWorkerClient
	resident   int
	inFlight   int
	known      map[string]bool
	lastUsed   time.Time
}

type GeometryPool struct {
	workerv1.UnimplementedGeometryWorkerServer
	ctx             context.Context
	config          GeometryPoolConfig
	mu              sync.Mutex
	workers         []*workerInstance
	nextPort        int
	debugAddress    string
	debugConnection *grpc.ClientConn
	debugClient     workerv1.GeometryWorkerClient
}

func NewGeometryPool(ctx context.Context, configuration GeometryPoolConfig) *GeometryPool {
	if configuration.MinimumWorkers < 1 {
		configuration.MinimumWorkers = 1
	}
	if configuration.MaximumWorkers < configuration.MinimumWorkers {
		configuration.MaximumWorkers = configuration.MinimumWorkers
	}
	if configuration.GeometryCapacity < 1 {
		configuration.GeometryCapacity = 2
	}
	if configuration.IdleTimeout <= 0 {
		configuration.IdleTimeout = 5 * time.Minute
	}
	return &GeometryPool{ctx: ctx, config: configuration, nextPort: configuration.FirstWorkerPort}
}

func (pool *GeometryPool) Start() error {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	for len(pool.workers) < pool.config.MinimumWorkers {
		if _, err := pool.spawnLocked(); err != nil {
			return err
		}
	}
	go pool.scaleDownLoop()
	return nil
}

func (pool *GeometryPool) Close() {
	pool.mu.Lock()
	workers := append([]*workerInstance{}, pool.workers...)
	pool.workers = nil
	debugConnection := pool.debugConnection
	pool.debugConnection = nil
	pool.mu.Unlock()
	if debugConnection != nil {
		_ = debugConnection.Close()
	}
	for _, worker := range workers {
		pool.stopWorker(worker)
	}
}

func (pool *GeometryPool) SetDebugAddress(address string) error {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if pool.debugConnection != nil {
		_ = pool.debugConnection.Close()
	}
	pool.debugAddress, pool.debugConnection, pool.debugClient = "", nil, nil
	if address == "" {
		return nil
	}
	connection, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	client := workerv1.NewGeometryWorkerClient(connection)
	pool.debugAddress, pool.debugConnection, pool.debugClient = address, connection, client
	slog.Info("geometry debug override enabled", "target", address)
	return nil
}

func (pool *GeometryPool) Status() map[string]any {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	workers := make([]map[string]any, 0, len(pool.workers))
	for _, worker := range pool.workers {
		workers = append(workers, map[string]any{"id": worker.id, "address": worker.address,
			"residentGeometry": worker.resident, "inFlight": worker.inFlight,
			"capacity": pool.config.GeometryCapacity, "lastUsed": worker.lastUsed})
	}
	return map[string]any{"workers": workers, "minimum": pool.config.MinimumWorkers,
		"maximum": pool.config.MaximumWorkers, "geometryCapacity": pool.config.GeometryCapacity,
		"debugOverride": pool.debugAddress}
}

func (pool *GeometryPool) spawnLocked() (*workerInstance, error) {
	port := pool.nextPort
	pool.nextPort++
	address := net.JoinHostPort(pool.config.WorkerHost, strconv.Itoa(port))
	environment := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "OCCCCAD_GEOMETRY_WORKER_LISTEN=") {
			environment = append(environment, entry)
		}
	}
	environment = append(environment, "OCCCCAD_GEOMETRY_WORKER_LISTEN="+address)
	process, err := StartManagedProcess(pool.ctx, fmt.Sprintf("geometry-%d", port), pool.config.WorkerBinary, "", nil, environment)
	if err != nil {
		return nil, err
	}
	connection, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		_ = process.Stop(2 * time.Second)
		return nil, err
	}
	client := workerv1.NewGeometryWorkerClient(connection)
	deadline := time.Now().Add(15 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(pool.ctx, 500*time.Millisecond)
		_, pingErr := client.Ping(ctx, &workerv1.PingRequest{})
		cancel()
		if pingErr == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = connection.Close()
			_ = process.Stop(2 * time.Second)
			return nil, fmt.Errorf("geometry worker %s did not become ready: %w", address, pingErr)
		}
		time.Sleep(100 * time.Millisecond)
	}
	worker := &workerInstance{id: fmt.Sprintf("geometry-%d", port), address: address,
		process: process, connection: connection, client: client, known: map[string]bool{}, lastUsed: time.Now()}
	pool.workers = append(pool.workers, worker)
	slog.Info("geometry worker added", "worker", worker.id, "address", address,
		"workers", len(pool.workers), "capacity", pool.config.GeometryCapacity)
	return worker, nil
}

func (pool *GeometryPool) stopWorker(worker *workerInstance) {
	_ = worker.connection.Close()
	_ = worker.process.Stop(5 * time.Second)
}

func (pool *GeometryPool) selectClient(geometryKey string) (workerv1.GeometryWorkerClient, *workerInstance, error) {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if pool.debugClient != nil {
		return pool.debugClient, nil, nil
	}
	var selected *workerInstance
	for _, worker := range pool.workers {
		if geometryKey != "" && worker.known[geometryKey] {
			selected = worker
			break
		}
		if worker.resident+worker.inFlight < pool.config.GeometryCapacity &&
			(selected == nil || worker.inFlight < selected.inFlight) {
			selected = worker
		}
	}
	if selected == nil && len(pool.workers) < pool.config.MaximumWorkers {
		worker, err := pool.spawnLocked()
		if err != nil {
			return nil, nil, err
		}
		selected = worker
	}
	if selected == nil {
		for _, worker := range pool.workers {
			if selected == nil || worker.inFlight < selected.inFlight {
				selected = worker
			}
		}
	}
	if selected == nil {
		return nil, nil, fmt.Errorf("no geometry worker is available")
	}
	selected.inFlight++
	selected.lastUsed = time.Now()
	return selected.client, selected, nil
}

func (pool *GeometryPool) release(worker *workerInstance, geometryKey string, successful bool) {
	if worker == nil {
		return
	}
	pool.mu.Lock()
	worker.inFlight--
	if successful && geometryKey != "" {
		worker.known[geometryKey] = true
	}
	pool.mu.Unlock()
	ctx, cancel := context.WithTimeout(pool.ctx, time.Second)
	response, err := worker.client.Ping(ctx, &workerv1.PingRequest{})
	cancel()
	if err == nil {
		pool.mu.Lock()
		worker.resident = int(response.GetResidentGeometryCount())
		pool.mu.Unlock()
		return
	}
	pool.removeFailedWorker(worker, err)
}

func (pool *GeometryPool) remember(worker *workerInstance, identifiers ...string) {
	if worker == nil {
		return
	}
	pool.mu.Lock()
	defer pool.mu.Unlock()
	for _, identifier := range identifiers {
		if identifier != "" {
			worker.known[identifier] = true
		}
	}
}

func (pool *GeometryPool) forget(worker *workerInstance, identifier string) {
	if worker == nil || identifier == "" {
		return
	}
	pool.mu.Lock()
	delete(worker.known, identifier)
	pool.mu.Unlock()
}

func (pool *GeometryPool) removeFailedWorker(worker *workerInstance, reason error) {
	pool.mu.Lock()
	removed := false
	for index, candidate := range pool.workers {
		if candidate == worker {
			pool.workers = append(pool.workers[:index], pool.workers[index+1:]...)
			removed = true
			break
		}
	}
	pool.mu.Unlock()
	if !removed {
		return
	}
	slog.Error("geometry worker failed", "worker", worker.id, "error", reason)
	pool.stopWorker(worker)
	pool.mu.Lock()
	defer pool.mu.Unlock()
	for len(pool.workers) < pool.config.MinimumWorkers {
		if _, err := pool.spawnLocked(); err != nil {
			slog.Error("replace geometry worker", "error", err)
			return
		}
	}
}

func outgoing(ctx context.Context) context.Context {
	if values, ok := metadata.FromIncomingContext(ctx); ok {
		return metadata.NewOutgoingContext(ctx, values.Copy())
	}
	return ctx
}

func (pool *GeometryPool) Ping(ctx context.Context, _ *workerv1.PingRequest) (*workerv1.PingResponse, error) {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	total := 0
	for _, worker := range pool.workers {
		total += worker.resident
	}
	return &workerv1.PingResponse{WorkerId: "occccad-geometry-router", OcctVersion: "routed",
		ResidentGeometryCount: uint32(total)}, nil
}

func (pool *GeometryPool) EvaluatePart(ctx context.Context, request *workerv1.EvaluatePartRequest) (*workerv1.EvaluatePartResponse, error) {
	client, worker, err := pool.selectClient(request.GetGeometryKey())
	if err != nil {
		return nil, err
	}
	response, err := client.EvaluatePart(outgoing(ctx), request)
	pool.release(worker, request.GetGeometryKey(), err == nil)
	if err == nil {
		pool.remember(worker, response.GetGeometryId())
	}
	return response, err
}
func (pool *GeometryPool) ImportStep(ctx context.Context, request *workerv1.ImportStepRequest) (*workerv1.EvaluatePartResponse, error) {
	client, worker, err := pool.selectClient(request.GetGeometryKey())
	if err != nil {
		return nil, err
	}
	response, err := client.ImportStep(outgoing(ctx), request)
	pool.release(worker, request.GetGeometryKey(), err == nil)
	if err == nil {
		pool.remember(worker, response.GetGeometryId())
	}
	return response, err
}
func (pool *GeometryPool) ExportStep(ctx context.Context, request *workerv1.ExportStepRequest) (*workerv1.ExportStepResponse, error) {
	client, worker, err := pool.selectClient("")
	if err != nil {
		return nil, err
	}
	response, err := client.ExportStep(outgoing(ctx), request)
	pool.release(worker, "", err == nil)
	return response, err
}
func (pool *GeometryPool) LoadGeometry(ctx context.Context, request *workerv1.LoadGeometryRequest) (*workerv1.LoadGeometryResponse, error) {
	client, worker, err := pool.selectClient("")
	if err != nil {
		return nil, err
	}
	response, err := client.LoadGeometry(outgoing(ctx), request)
	pool.release(worker, "", err == nil)
	if err == nil {
		pool.remember(worker, response.GetGeometryId())
	}
	return response, err
}
func (pool *GeometryPool) UnloadGeometry(ctx context.Context, request *workerv1.UnloadGeometryRequest) (*workerv1.UnloadGeometryResponse, error) {
	client, worker, err := pool.selectClient(request.GetGeometryId())
	if err != nil {
		return nil, err
	}
	response, err := client.UnloadGeometry(outgoing(ctx), request)
	pool.release(worker, "", err == nil)
	if err == nil {
		pool.forget(worker, request.GetGeometryId())
	}
	return response, err
}
func (pool *GeometryPool) GetTopology(ctx context.Context, request *workerv1.GetTopologyRequest) (*workerv1.GetTopologyResponse, error) {
	client, worker, err := pool.selectClient(request.GetGeometryId())
	if err != nil {
		return nil, err
	}
	response, err := client.GetTopology(outgoing(ctx), request)
	pool.release(worker, "", err == nil)
	return response, err
}
func (pool *GeometryPool) Tessellate(ctx context.Context, request *workerv1.TessellateRequest) (*workerv1.TessellateResponse, error) {
	client, worker, err := pool.selectClient(request.GetGeometryId())
	if err != nil {
		return nil, err
	}
	response, err := client.Tessellate(outgoing(ctx), request)
	pool.release(worker, "", err == nil)
	return response, err
}
func (pool *GeometryPool) CreateChamfer(ctx context.Context, request *workerv1.CreateChamferRequest) (*workerv1.CreateChamferResponse, error) {
	client, worker, err := pool.selectClient(request.GetGeometryId())
	if err != nil {
		return nil, err
	}
	response, err := client.CreateChamfer(outgoing(ctx), request)
	pool.release(worker, "", err == nil)
	if err == nil {
		pool.remember(worker, response.GetNewGeometryId())
	}
	return response, err
}
func (pool *GeometryPool) CreateFillet(ctx context.Context, request *workerv1.CreateFilletRequest) (*workerv1.CreateFilletResponse, error) {
	client, worker, err := pool.selectClient(request.GetGeometryId())
	if err != nil {
		return nil, err
	}
	response, err := client.CreateFillet(outgoing(ctx), request)
	pool.release(worker, "", err == nil)
	if err == nil {
		pool.remember(worker, response.GetNewGeometryId())
	}
	return response, err
}

func (pool *GeometryPool) scaleDownLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-pool.ctx.Done():
			return
		case <-ticker.C:
			pool.scaleDown()
		}
	}
}
func (pool *GeometryPool) scaleDown() {
	pool.mu.Lock()
	if len(pool.workers) <= pool.config.MinimumWorkers {
		pool.mu.Unlock()
		return
	}
	var removed *workerInstance
	for index := len(pool.workers) - 1; index >= pool.config.MinimumWorkers; index-- {
		candidate := pool.workers[index]
		if candidate.inFlight == 0 && time.Since(candidate.lastUsed) >= pool.config.IdleTimeout {
			removed = candidate
			pool.workers = append(pool.workers[:index], pool.workers[index+1:]...)
			break
		}
	}
	pool.mu.Unlock()
	if removed != nil {
		pool.stopWorker(removed)
		slog.Info("idle geometry worker removed", "worker", removed.id)
	}
}
