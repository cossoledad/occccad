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
	Configured    bool           `json:"configured"`
	Provider      string         `json:"provider"`
	Model         string         `json:"model"`
	Thinking      string         `json:"thinking"`
	Budget        string         `json:"budget"`
	BudgetPresets []BudgetPreset `json:"budgetPresets"`
}

func (s *Service) Settings(ctx context.Context, actor string) (Settings, error) {
	value := Settings{Provider: "DeepSeek", Model: s.options.Model, Thinking: "off", Budget: defaultBudget, BudgetPresets: budgetPresets()}
	var model, thinking, budget string
	err := s.db.QueryRow(ctx, `SELECT model,thinking,budget FROM occccad.assistant_credentials WHERE actor_id=$1`, actor).Scan(&model, &thinking, &budget)
	if errors.Is(err, database.ErrNoRows) {
		return value, nil
	}
	if err != nil {
		return value, err
	}
	value.Configured = true
	if model != "" {
		value.Model = model
	}
	value.Thinking = thinking
	value.Budget = budget
	return value, nil
}

func (s *Service) Models(ctx context.Context, actor string) ([]Model, error) {
	key, err := s.credential(ctx, actor)
	if err != nil {
		return nil, err
	}
	return s.provider.Models(ctx, key)
}

func (s *Service) SavePreferences(ctx context.Context, actor string, generation Generation) (Settings, error) {
	previous, err := s.Settings(ctx, actor)
	if err != nil {
		return Settings{}, err
	}
	if !previous.Configured {
		return Settings{}, aiFault("AI_KEY_REQUIRED", "请先连接 DeepSeek", false)
	}
	if generation.Budget == "" {
		generation.Budget = previous.Budget
	}
	if _, err = budgetFor(generation.Budget); err != nil {
		return Settings{}, err
	}
	if !validThinking(generation.Thinking) {
		return Settings{}, aiFault("AI_CONFIGURATION", "请选择有效的思考度", false)
	}
	// A local budget change must work even when model discovery is unavailable.
	if generation.Model != previous.Model || generation.Thinking != previous.Thinking {
		models, err := s.Models(ctx, actor)
		if err != nil {
			return Settings{}, err
		}
		found := false
		for _, model := range models {
			if model.ID == generation.Model {
				found = true
				if model.Effort != nil && generation.Thinking != "off" {
					supported := false
					for _, level := range model.Effort.SupportedLevels {
						supported = supported || level == generation.Thinking
					}
					if !supported {
						return Settings{}, aiFault("AI_CONFIGURATION", "所选模型不支持这个思考度", false)
					}
				}
			}
		}
		if !found {
			return Settings{}, aiFault("AI_MODEL_UNAVAILABLE", "当前 API Key 无法访问所选模型", false)
		}
	}
	_, err = s.db.Exec(ctx, `UPDATE occccad.assistant_credentials SET model=$1,thinking=$2,budget=$3,updated_at=now() WHERE actor_id=$4`, generation.Model, generation.Thinking, generation.Budget, actor)
	if err != nil {
		return Settings{}, err
	}
	return s.Settings(ctx, actor)
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
	models, err := s.provider.Models(ctx, key)
	if err != nil {
		return Settings{}, err
	}
	model := models[0].ID
	for _, candidate := range models {
		if candidate.ID == s.options.Model {
			model = candidate.ID
		}
	}
	previous, err := s.Settings(ctx, actor)
	if err != nil {
		return Settings{}, err
	}
	thinking := "off"
	for _, candidate := range models {
		if candidate.ID != previous.Model {
			continue
		}
		model = candidate.ID
		thinking = previous.Thinking
		if candidate.Effort != nil && thinking != "off" {
			supported := false
			for _, level := range candidate.Effort.SupportedLevels {
				supported = supported || level == thinking
			}
			if !supported {
				thinking = "off"
			}
		}
		break
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
	_, err = s.db.Exec(ctx, `INSERT INTO occccad.assistant_credentials(actor_id,ciphertext,model,thinking) VALUES($1,$2,$3,$4) ON CONFLICT(actor_id) DO UPDATE SET ciphertext=excluded.ciphertext,model=excluded.model,thinking=excluded.thinking,updated_at=now()`, actor, encrypted, model, thinking)
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
