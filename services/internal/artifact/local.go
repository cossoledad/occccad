package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type LocalStore struct {
	root string
}

func NewLocalStore(root string) (*LocalStore, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve artifact directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(absolute, "artifacts", "sha256"), 0o750); err != nil {
		return nil, fmt.Errorf("create artifact directory: %w", err)
	}
	return &LocalStore{root: absolute}, nil
}

func (store *LocalStore) Root() string { return store.root }

func (store *LocalStore) Put(ctx context.Context, kind Kind, contentType string, source io.Reader) (StoredObject, error) {
	if err := ctx.Err(); err != nil {
		return StoredObject{}, err
	}
	temporaryDirectory := filepath.Join(store.root, "artifacts", ".staging")
	if err := os.MkdirAll(temporaryDirectory, 0o750); err != nil {
		return StoredObject{}, err
	}
	temporary, err := os.CreateTemp(temporaryDirectory, "upload-*")
	if err != nil {
		return StoredObject{}, err
	}
	temporaryName := temporary.Name()
	defer func() { _ = os.Remove(temporaryName) }()

	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(temporary, hash), source)
	if err != nil {
		_ = temporary.Close()
		return StoredObject{}, err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return StoredObject{}, err
	}
	if err := temporary.Close(); err != nil {
		return StoredObject{}, err
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	key := filepath.ToSlash(filepath.Join("artifacts", "sha256", digest[:2], digest, fileName(kind)))
	target, err := store.resolve(key)
	if err != nil {
		return StoredObject{}, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return StoredObject{}, err
	}
	if info, err := os.Stat(target); err == nil {
		if info.Size() != size {
			return StoredObject{}, fmt.Errorf("artifact collision for %s", digest)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return StoredObject{}, err
	} else if err := os.Link(temporaryName, target); err != nil {
		// Another process may have completed the same content-addressed write.
		if info, statErr := os.Stat(target); statErr != nil || info.Size() != size {
			return StoredObject{}, err
		}
	}
	return StoredObject{Key: key, SHA256: digest, Size: size, ContentType: contentType}, nil
}

func (store *LocalStore) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := store.resolve(key)
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}

func (store *LocalStore) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := store.resolve(key)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (store *LocalStore) resolve(key string) (string, error) {
	if key == "" || filepath.IsAbs(key) || strings.Contains(key, "\\") {
		return "", errors.New("invalid artifact key")
	}
	clean := filepath.Clean(filepath.FromSlash(key))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("invalid artifact key")
	}
	path := filepath.Join(store.root, clean)
	relative, err := filepath.Rel(store.root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("artifact key escapes storage root")
	}
	return path, nil
}

func fileName(kind Kind) string {
	switch kind {
	case KindBREP:
		return "shape.brep"
	case KindGLB:
		return "mesh.glb"
	case KindExchangeSource:
		return "source.exchange"
	case KindExchangeExport:
		return "export.exchange"
	case KindThumbnail:
		return "preview.svg"
	default:
		return "object.bin"
	}
}
