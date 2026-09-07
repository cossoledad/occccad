package debugartifact

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrNotFound = errors.New("debug artifact not found")

type Item struct {
	ID        string `json:"id"`
	RequestID string `json:"requestId"`
	CreatedAt string `json:"createdAt"`
	Status    string `json:"status"`
}

type metadata struct {
	Item
	DocumentID string `json:"documentId"`
	File       string `json:"file"`
	Bytes      int    `json:"bytes"`
}

// Store is a bounded, process-independent local repository for downloadable
// diagnostic evidence. It is intentionally not part of model history.
type Store struct {
	root      string
	maxPerDoc int
	maxAge    time.Duration
	mu        sync.Mutex
	now       func() time.Time
}

func NewStore(root string, maxPerDocument int, maxAge time.Duration) (*Store, error) {
	if maxPerDocument < 1 {
		maxPerDocument = 50
	}
	if maxAge <= 0 {
		maxAge = 7 * 24 * time.Hour
	}
	root = filepath.Clean(root)
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, fmt.Errorf("create debug artifact directory: %w", err)
	}
	return &Store{root: root, maxPerDoc: maxPerDocument, maxAge: maxAge, now: time.Now}, nil
}

func (store *Store) Save(ctx context.Context, documentID, requestID string, data []byte) (Item, error) {
	if err := ctx.Err(); err != nil {
		return Item{}, err
	}
	if !safeID(documentID) || strings.TrimSpace(requestID) == "" || len(data) == 0 {
		return Item{}, errors.New("invalid debug artifact identity or content")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	directory := filepath.Join(store.root, documentID)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return Item{}, err
	}
	created := store.now().UTC()
	id := created.Format("20060102T150405.000000000Z") + "-" + randomID()
	item := Item{ID: id, RequestID: requestID, CreatedAt: created.Format(time.RFC3339Nano), Status: replayStatus(data)}
	meta := metadata{Item: item, DocumentID: documentID, File: id + ".3dreplay", Bytes: len(data)}
	if err := writeAtomic(filepath.Join(directory, meta.File), data, 0o640); err != nil {
		return Item{}, err
	}
	encoded, err := json.Marshal(meta)
	if err != nil {
		return Item{}, err
	}
	if err = writeAtomic(filepath.Join(directory, id+".meta.json"), encoded, 0o640); err != nil {
		_ = os.Remove(filepath.Join(directory, meta.File))
		return Item{}, err
	}
	store.pruneLocked(directory)
	return item, nil
}

func (store *Store) List(ctx context.Context, documentID string, limit int) ([]Item, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !safeID(documentID) {
		return nil, ErrNotFound
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	all, err := store.metadataLocked(filepath.Join(store.root, documentID))
	if err != nil {
		return nil, err
	}
	if limit < 1 || limit > store.maxPerDoc {
		limit = store.maxPerDoc
	}
	if len(all) > limit {
		all = all[:limit]
	}
	result := make([]Item, 0, len(all))
	for _, value := range all {
		result = append(result, value.Item)
	}
	return result, nil
}

func (store *Store) Read(ctx context.Context, documentID, id, requestID string) (Item, []byte, error) {
	if err := ctx.Err(); err != nil {
		return Item{}, nil, err
	}
	if !safeID(documentID) {
		return Item{}, nil, ErrNotFound
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	all, err := store.metadataLocked(filepath.Join(store.root, documentID))
	if err != nil {
		return Item{}, nil, err
	}
	for _, value := range all {
		if (id == "latest" || value.ID == id) && (requestID == "" || value.RequestID == requestID) {
			data, readErr := os.ReadFile(filepath.Join(store.root, documentID, value.File))
			if readErr != nil {
				if errors.Is(readErr, os.ErrNotExist) {
					return Item{}, nil, ErrNotFound
				}
				return Item{}, nil, readErr
			}
			return value.Item, data, nil
		}
	}
	return Item{}, nil, ErrNotFound
}

func (store *Store) metadataLocked(directory string) ([]metadata, error) {
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return []metadata{}, nil
	}
	if err != nil {
		return nil, err
	}
	all := make([]metadata, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".meta.json") {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(directory, entry.Name()))
		if readErr != nil {
			continue
		}
		var value metadata
		if json.Unmarshal(data, &value) == nil && value.ID != "" {
			all = append(all, value)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].CreatedAt > all[j].CreatedAt })
	return all, nil
}

func (store *Store) pruneLocked(directory string) {
	all, err := store.metadataLocked(directory)
	if err != nil {
		return
	}
	cutoff := store.now().Add(-store.maxAge)
	for index, value := range all {
		created, _ := time.Parse(time.RFC3339Nano, value.CreatedAt)
		if index < store.maxPerDoc && (created.IsZero() || created.After(cutoff)) {
			continue
		}
		_ = os.Remove(filepath.Join(directory, value.File))
		_ = os.Remove(filepath.Join(directory, value.ID+".meta.json"))
	}
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	temporary := path + ".tmp-" + randomID()
	if err := os.WriteFile(temporary, data, mode); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func safeID(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}
	for _, char := range value {
		if !(char == '-' || char >= '0' && char <= '9' || char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z') {
			return false
		}
	}
	return true
}

func randomID() string {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer)
}

func replayStatus(data []byte) string {
	var value struct {
		Result struct {
			Status string `json:"status"`
		} `json:"result"`
	}
	if json.Unmarshal(data, &value) != nil || value.Result.Status == "" {
		return "TRANSPORT_ERROR"
	}
	return value.Result.Status
}
