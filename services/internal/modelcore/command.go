// Package modelcore contains the OCCT-free, deterministic model editing core.
package modelcore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	ErrUnsupportedCommand = errors.New("COMMAND_SCHEMA_UNSUPPORTED")
	ErrInvalidCommand     = errors.New("INVALID_DOMAIN_COMMAND")
)

type DomainCommand struct {
	CommandID     string          `json:"commandId"`
	TypeURI       string          `json:"typeUri"`
	SchemaVersion uint32          `json:"schemaVersion"`
	Payload       json.RawMessage `json:"payload"`
}

type DomainTransaction struct {
	TransactionID          string          `json:"transactionId"`
	RequestID              string          `json:"requestId"`
	DocumentID             string          `json:"documentId"`
	WorkspaceID            string          `json:"workspaceId"`
	ExpectedHeadSequence   uint64          `json:"expectedHeadSequence"`
	ExpectedHeadRevisionID string          `json:"expectedHeadRevisionId"`
	Commands               []DomainCommand `json:"commands"`
	EvaluationPolicy       string          `json:"evaluationPolicy"`
}

func (transaction DomainTransaction) Validate() error {
	if strings.TrimSpace(transaction.TransactionID) == "" || strings.TrimSpace(transaction.RequestID) == "" {
		return fmt.Errorf("%w: transaction_id and request_id are required", ErrInvalidCommand)
	}
	if strings.TrimSpace(transaction.DocumentID) == "" || strings.TrimSpace(transaction.WorkspaceID) == "" {
		return fmt.Errorf("%w: document_id and workspace_id are required", ErrInvalidCommand)
	}
	if len(transaction.Commands) == 0 || len(transaction.Commands) > 64 {
		return fmt.Errorf("%w: a transaction requires 1..64 commands", ErrInvalidCommand)
	}
	seen := map[string]struct{}{}
	for _, command := range transaction.Commands {
		if err := command.Validate(); err != nil {
			return err
		}
		if _, exists := seen[command.CommandID]; exists {
			return fmt.Errorf("%w: duplicate command id %q", ErrInvalidCommand, command.CommandID)
		}
		seen[command.CommandID] = struct{}{}
	}
	return nil
}

func (command DomainCommand) Validate() error {
	if strings.TrimSpace(command.CommandID) == "" || strings.TrimSpace(command.TypeURI) == "" {
		return fmt.Errorf("%w: command_id and type_uri are required", ErrInvalidCommand)
	}
	if command.SchemaVersion == 0 || len(command.Payload) == 0 || !json.Valid(command.Payload) {
		return fmt.Errorf("%w: schema_version and valid JSON payload are required", ErrInvalidCommand)
	}
	return nil
}

func (transaction DomainTransaction) CanonicalDigest() (string, error) {
	if err := transaction.Validate(); err != nil {
		return "", err
	}
	data, err := json.Marshal(transaction)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

type Handler interface {
	TypeURI() string
	SupportedSchemaVersions() []uint32
	TargetDocumentTypes() []string
	Apply(model json.RawMessage, payload json.RawMessage) (json.RawMessage, ChangeSet, error)
}

type Registry struct {
	handlers map[string]Handler
}

func NewRegistry(handlers ...Handler) (*Registry, error) {
	registry := &Registry{handlers: map[string]Handler{}}
	for _, handler := range handlers {
		if handler == nil || strings.TrimSpace(handler.TypeURI()) == "" {
			return nil, fmt.Errorf("handler type URI is required")
		}
		if _, exists := registry.handlers[handler.TypeURI()]; exists {
			return nil, fmt.Errorf("duplicate command handler %q", handler.TypeURI())
		}
		registry.handlers[handler.TypeURI()] = handler
	}
	return registry, nil
}

func (registry *Registry) Apply(documentType string, model json.RawMessage, command DomainCommand) (json.RawMessage, ChangeSet, error) {
	if err := command.Validate(); err != nil {
		return nil, ChangeSet{}, err
	}
	handler, exists := registry.handlers[command.TypeURI]
	if !exists || !containsVersion(handler.SupportedSchemaVersions(), command.SchemaVersion) ||
		!containsStringFold(handler.TargetDocumentTypes(), documentType) {
		return nil, ChangeSet{}, fmt.Errorf("%w: %s v%d for %s", ErrUnsupportedCommand,
			command.TypeURI, command.SchemaVersion, documentType)
	}
	next, changes, err := handler.Apply(append(json.RawMessage(nil), model...), command.Payload)
	if err != nil {
		return nil, ChangeSet{}, err
	}
	if !json.Valid(next) {
		return nil, ChangeSet{}, fmt.Errorf("handler %s returned invalid model JSON", command.TypeURI)
	}
	if err := changes.Finalize(); err != nil {
		return nil, ChangeSet{}, fmt.Errorf("handler %s returned invalid change set: %w", command.TypeURI, err)
	}
	return next, changes, nil
}

func (registry *Registry) Types() []string {
	result := make([]string, 0, len(registry.handlers))
	for typeURI := range registry.handlers {
		result = append(result, typeURI)
	}
	sort.Strings(result)
	return result
}

func containsVersion(values []uint32, target uint32) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsStringFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}
