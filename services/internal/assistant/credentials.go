package assistant

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/config"
	"github.com/occccad/occccad/internal/database"
)

type Options struct {
	BaseURL string
	Model   string
	KeyFile string
}

func DefaultOptions() Options {
	value := func(name, fallback string) string {
		if v := os.Getenv(name); v != "" {
			return v
		}
		return fallback
	}
	return Options{
		BaseURL: value("OCCCCAD_AI_BASE_URL", "https://api.deepseek.com"),
		Model:   value("OCCCCAD_AI_MODEL", "deepseek-flash"),
		KeyFile: value("OCCCCAD_AI_KEY_FILE", filepath.Join(config.Load().DataDirectory, "assistant", "credentials.key")),
	}
}

type Service struct {
	db         database.DB
	automation *automation.Service
	provider   ChatProvider
	options    Options
	keyMu      sync.Mutex
	aead       cipher.AEAD
	runMu      sync.Mutex
	runs       map[string]context.CancelFunc
}

func New(db database.DB, operations *automation.Service, options Options) *Service {
	return &Service{db: db, automation: operations, options: options, provider: newProvider(options.BaseURL, options.Model), runs: map[string]context.CancelFunc{}}
}

type Settings struct {
	Configured bool   `json:"configured"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
}

func (s *Service) Settings(ctx context.Context, actor string) (Settings, error) {
	var count int
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM occccad.assistant_credentials WHERE actor_id=$1`, actor).Scan(&count)
	return Settings{Configured: count > 0, Provider: "DeepSeek", Model: s.options.Model}, err
}

func (s *Service) cipher() (cipher.AEAD, error) {
	s.keyMu.Lock()
	defer s.keyMu.Unlock()
	if s.aead != nil {
		return s.aead, nil
	}
	path := s.options.KeyFile
	key, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}
		key = make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return nil, err
		}
		// Publish a complete 0600 file atomically; concurrent API processes share
		// the winning key rather than overwrite credentials with another key.
		file, e := os.CreateTemp(filepath.Dir(path), ".credentials-*")
		if e != nil {
			return nil, e
		}
		defer os.Remove(file.Name())
		_, err = file.Write(key)
		if err == nil {
			err = file.Sync()
		}
		closeErr := file.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if err = os.Link(file.Name(), path); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		key, err = os.ReadFile(path)
	}
	if err != nil || len(key) != 32 {
		return nil, errors.New("credential encryption key unavailable")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	s.aead, err = cipher.NewGCM(block)
	return s.aead, err
}
func (s *Service) SaveKey(ctx context.Context, actor, key string) (Settings, error) {
	key = strings.TrimSpace(key)
	if len(key) < 8 || len(key) > 4096 || strings.ContainsAny(key, "\r\n\t ") {
		return Settings{}, aiFault("AI_INVALID_KEY", "请输入有效的 API Key", false)
	}
	if err := s.provider.Validate(ctx, key); err != nil {
		return Settings{}, err
	}
	box, err := s.cipher()
	if err != nil {
		return Settings{}, aiFault("AI_CREDENTIAL_STORE", "无法保存连接密钥，请检查服务端密钥文件权限", false)
	}
	nonce := make([]byte, box.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return Settings{}, err
	}
	encrypted := base64.RawStdEncoding.EncodeToString(box.Seal(nonce, nonce, []byte(key), []byte(actor)))
	_, err = s.db.Exec(ctx, `INSERT INTO occccad.assistant_credentials(actor_id,ciphertext) VALUES($1,$2) ON CONFLICT(actor_id) DO UPDATE SET ciphertext=excluded.ciphertext,updated_at=now()`, actor, encrypted)
	if err != nil {
		return Settings{}, err
	}
	return s.Settings(ctx, actor)
}
func (s *Service) DeleteKey(ctx context.Context, actor string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM occccad.assistant_credentials WHERE actor_id=$1`, actor)
	return err
}
func (s *Service) credential(ctx context.Context, actor string) (string, error) {
	var raw string
	err := s.db.QueryRow(ctx, `SELECT ciphertext FROM occccad.assistant_credentials WHERE actor_id=$1`, actor).Scan(&raw)
	if errors.Is(err, database.ErrNoRows) {
		return "", aiFault("AI_KEY_REQUIRED", "请先连接 DeepSeek：填写你的 API Key 即可开始对话建模", false)
	}
	if err != nil {
		return "", err
	}
	box, err := s.cipher()
	if err != nil {
		return "", aiFault("AI_CREDENTIAL_STORE", "连接密钥不可读取，请重新设置 API Key", false)
	}
	encrypted, err := base64.RawStdEncoding.DecodeString(raw)
	if err == nil && len(encrypted) >= box.NonceSize() {
		var plain []byte
		plain, err = box.Open(nil, encrypted[:box.NonceSize()], encrypted[box.NonceSize():], []byte(actor))
		if err == nil {
			return string(plain), nil
		}
	}
	return "", aiFault("AI_CREDENTIAL_STORE", "连接密钥不可读取，请重新设置 API Key", false)
}
