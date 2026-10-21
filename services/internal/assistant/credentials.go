package assistant

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/config"
	"github.com/occccad/occccad/internal/database"
)

type Options struct {
	BaseURL string
	Model   string
	KeyFile string
	Now     func() time.Time
	Limits  RequestLimits
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
	runActors  map[string]string
	wake       chan struct{}
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	sessions   map[string]*planningSession
}

func New(db database.DB, operations *automation.Service, options Options) *Service {
	return &Service{db: db, automation: operations, options: options, provider: newProvider(options.BaseURL, options.Model), runs: map[string]context.CancelFunc{}, runActors: map[string]string{}, wake: make(chan struct{}, 1), sessions: map[string]*planningSession{}}
}

type Settings struct {
	Configured     bool          `json:"configured"`
	Provider       string        `json:"provider"`
	ProviderOrigin string        `json:"providerOrigin"`
	Model          string        `json:"model"`
	Thinking       string        `json:"thinking"`
	TokenBudget    *int64        `json:"tokenBudget"`
	Version        uint64        `json:"version"`
	RequestLimits  RequestLimits `json:"requestLimits"`
}

func (s *Service) requestLimits() RequestLimits {
	if s.options.Limits.MaxContextBytes > 0 {
		return s.options.Limits
	}
	return defaultRequestLimits()
}
func (s *Service) Settings(ctx context.Context, actor string) (Settings, error) {
	value := Settings{Provider: "DeepSeek", ProviderOrigin: s.providerOrigin(), Model: s.options.Model, Thinking: "off", RequestLimits: s.requestLimits()}
	var model string
	err := s.db.QueryRow(ctx, `SELECT model,thinking,token_budget,settings_version FROM occccad.assistant_credentials WHERE actor_id=$1`, actor).Scan(&model, &value.Thinking, &value.TokenBudget, &value.Version)
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
	return value, nil
}
func (s *Service) Models(ctx context.Context, actor string) ([]Model, error) {
	key, err := s.credential(ctx, actor)
	if err != nil {
		return nil, err
	}
	return s.provider.Models(ctx, key)
}

func (s *Service) SavePreferences(ctx context.Context, actor string, generation Generation, budgets ...*int64) (Settings, error) {
	previous, err := s.Settings(ctx, actor)
	if err != nil {
		return Settings{}, err
	}
	if !previous.Configured {
		return Settings{}, aiFault("AI_KEY_REQUIRED", "请先连接 DeepSeek", false)
	}
	tokenBudget := previous.TokenBudget
	if len(budgets) > 0 {
		tokenBudget = budgets[0]
	}
	if !validTokenBudget(tokenBudget) {
		return Settings{}, aiFault("AI_CONFIGURATION", "累计 Token 预算须为正整数，留空表示无累计上限", false)
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
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Settings{}, err
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `UPDATE occccad.assistant_credentials SET model=$1,thinking=$2,token_budget=$3,settings_version=settings_version+1,updated_at=now() WHERE actor_id=$4 AND settings_version=$5`, generation.Model, generation.Thinking, tokenBudget, actor, previous.Version)
	if err != nil {
		return Settings{}, err
	}
	if result.RowsAffected() != 1 {
		return Settings{}, aiFault("AI_SETTINGS_CONFLICT", "模型设置已变化，请重新读取后再保存", true)
	}
	next := previous
	next.Model = generation.Model
	next.Thinking = generation.Thinking
	next.TokenBudget = tokenBudget
	next.Version++
	if _, err = tx.Exec(ctx, `INSERT INTO occccad.assistant_settings_events(actor_id,version,payload) VALUES($1,$2,$3)`, actor, next.Version, marshalJSON(next)); err != nil {
		return Settings{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Settings{}, err
	}
	return next, nil
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
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Settings{}, err
	}
	defer tx.Rollback(ctx)
	value := Settings{Configured: true, Provider: "DeepSeek", ProviderOrigin: s.providerOrigin(), Model: model, Thinking: thinking, RequestLimits: s.requestLimits()}
	err = tx.QueryRow(ctx, `INSERT INTO occccad.assistant_credentials(actor_id,ciphertext,model,thinking) VALUES($1,$2,$3,$4) ON CONFLICT(actor_id) DO UPDATE SET ciphertext=excluded.ciphertext,model=excluded.model,thinking=excluded.thinking,settings_version=assistant_credentials.settings_version+1,updated_at=now() RETURNING settings_version,token_budget`, actor, encrypted, model, thinking).Scan(&value.Version, &value.TokenBudget)
	if err != nil {
		return Settings{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO occccad.assistant_settings_events(actor_id,version,payload) VALUES($1,$2,$3)`, actor, value.Version, marshalJSON(map[string]any{"source": "USER_CONNECTION", "settings": value, "at": timestamp(s.now())})); err != nil {
		return Settings{}, err
	}
	return value, tx.Commit(ctx)
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

// Deployment encryption-store identity fences schedulers sharing the same development DB.
// It is not a CAD identity; no path or credential enters the public model.
func (s *Service) vaultScope() (string, error) {
	if _, err := s.cipher(); err != nil {
		return "", err
	}
	raw, err := os.ReadFile(s.options.KeyFile)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(append([]byte("occccad/assistant-vault/v2:"+s.providerOrigin()+":"), raw...))
	return hex.EncodeToString(digest[:]), nil
}

func (s *Service) providerOrigin() string {
	u, e := url.Parse(s.options.BaseURL)
	if e != nil || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
