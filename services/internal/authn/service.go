package authn

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/occccad/occccad/internal/access"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUnauthorized = errors.New("invalid email or password")
	ErrForbidden    = errors.New("account is not active")
	ErrValidation   = errors.New("invalid account request")
	ErrNotFound     = errors.New("account not found")
)

type User struct {
	ID                 string  `json:"id"`
	Email              string  `json:"email"`
	DisplayName        string  `json:"displayName"`
	Status             string  `json:"status"`
	PlatformRole       string  `json:"platformRole"`
	MustChangePassword bool    `json:"mustChangePassword"`
	CreatedAt          string  `json:"createdAt"`
	LastLoginAt        *string `json:"lastLoginAt,omitempty"`
}

func (user User) Principal() access.User {
	return access.User{ID: user.ID, Email: user.Email, DisplayName: user.DisplayName,
		Status: user.Status, PlatformRole: user.PlatformRole, MustChangePassword: user.MustChangePassword}
}

type Session struct {
	User      User
	Token     string
	CSRFToken string
	ExpiresAt time.Time
}

type RegisterRequest struct {
	Email, DisplayName, Password string
}

type CreateUserRequest struct {
	Email, DisplayName, Password, PlatformRole, Status string
}

type UpdateUserRequest struct {
	DisplayName, PlatformRole, Status string
}

type Service struct {
	database        *pgxpool.Pool
	sessionDuration time.Duration
}

func New(database *pgxpool.Pool, sessionDuration time.Duration) *Service {
	if sessionDuration <= 0 {
		sessionDuration = 12 * time.Hour
	}
	return &Service{database: database, sessionDuration: sessionDuration}
}

func normalizeEmail(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	parsed, err := mail.ParseAddress(value)
	if err != nil || parsed.Address != value || len(value) > 254 {
		return "", fmt.Errorf("%w: email is invalid", ErrValidation)
	}
	return value, nil
}

func validatePassword(value string) error {
	if len(value) < 10 || len(value) > 128 {
		return fmt.Errorf("%w: password must contain 10 to 128 characters", ErrValidation)
	}
	return nil
}

func validateDisplayName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) < 1 || len(value) > 120 {
		return "", fmt.Errorf("%w: display name is required and must not exceed 120 characters", ErrValidation)
	}
	return value, nil
}

func (service *Service) BootstrapAdmin(ctx context.Context, email, displayName, password string) error {
	email, err := normalizeEmail(email)
	if err != nil {
		return fmt.Errorf("administrator email: %w", err)
	}
	displayName, err = validateDisplayName(displayName)
	if err != nil {
		return fmt.Errorf("administrator display name: %w", err)
	}
	if err := validatePassword(password); err != nil {
		return fmt.Errorf("administrator password: %w", err)
	}
	var currentHash *string
	err = service.database.QueryRow(ctx, `SELECT password_hash FROM occccad.users WHERE id=$1`, access.DefaultUserID).
		Scan(&currentHash)
	if err != nil {
		return err
	}
	passwordHash := currentHash
	initializedPassword := false
	if passwordHash == nil || *passwordHash == "" {
		encoded, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		value := string(encoded)
		passwordHash = &value
		initializedPassword = true
	}
	_, err = service.database.Exec(ctx, `UPDATE occccad.users SET
		email=$1,display_name=$2,password_hash=$3,status='ACTIVE',platform_role='ADMIN',
		approved_at=COALESCE(approved_at,now()),
		must_change_password=CASE WHEN $5 THEN true ELSE must_change_password END,updated_at=now()
		WHERE id=$4`, email, displayName, passwordHash, access.DefaultUserID, initializedPassword)
	return err
}

func (service *Service) Register(ctx context.Context, request RegisterRequest) (User, error) {
	email, err := normalizeEmail(request.Email)
	if err != nil {
		return User{}, err
	}
	name, err := validateDisplayName(request.DisplayName)
	if err != nil {
		return User{}, err
	}
	if err := validatePassword(request.Password); err != nil {
		return User{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(request.Password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}
	var result User
	err = service.database.QueryRow(ctx, `INSERT INTO occccad.users(
		email,display_name,status,platform_role,password_hash)
		VALUES($1,$2,'PENDING','MEMBER',$3)
		RETURNING id::text,email,display_name,status,platform_role,must_change_password,created_at::text,NULL`,
		email, name, string(hash)).Scan(&result.ID, &result.Email, &result.DisplayName, &result.Status,
		&result.PlatformRole, &result.MustChangePassword, &result.CreatedAt, &result.LastLoginAt)
	if err != nil && strings.Contains(err.Error(), "users_email_idx") {
		return User{}, fmt.Errorf("%w: email is already registered", ErrValidation)
	}
	return result, err
}

func (service *Service) Login(ctx context.Context, email, password, userAgent, remoteAddress string) (Session, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return Session{}, ErrUnauthorized
	}
	var user User
	var passwordHash *string
	var lockedUntil *time.Time
	err = service.database.QueryRow(ctx, `SELECT id::text,email,display_name,status,platform_role,
		must_change_password,created_at::text,NULL,password_hash,locked_until
		FROM occccad.users WHERE lower(email)=lower($1)`, email).Scan(&user.ID, &user.Email,
		&user.DisplayName, &user.Status, &user.PlatformRole, &user.MustChangePassword,
		&user.CreatedAt, &user.LastLoginAt, &passwordHash, &lockedUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$7EqJtq98hPqEX7fNZaFWoOhiLK8TjViQeS2p4V0O9hCq6H6hEXR8K"), []byte(password))
		return Session{}, ErrUnauthorized
	}
	if err != nil {
		return Session{}, err
	}
	if lockedUntil != nil && lockedUntil.After(time.Now()) {
		return Session{}, ErrUnauthorized
	}
	if passwordHash == nil || bcrypt.CompareHashAndPassword([]byte(*passwordHash), []byte(password)) != nil {
		_, _ = service.database.Exec(ctx, `UPDATE occccad.users SET
			failed_login_count=failed_login_count+1,
			locked_until=CASE WHEN failed_login_count+1>=5 THEN now()+interval '15 minutes' ELSE locked_until END
			WHERE id=$1`, user.ID)
		return Session{}, ErrUnauthorized
	}
	if user.Status != "ACTIVE" {
		return Session{}, ErrForbidden
	}
	token, err := randomToken()
	if err != nil {
		return Session{}, err
	}
	csrf, err := randomToken()
	if err != nil {
		return Session{}, err
	}
	expires := time.Now().Add(service.sessionDuration)
	_, err = service.database.Exec(ctx, `WITH session AS (
		INSERT INTO occccad.user_sessions(user_id,token_hash,csrf_hash,user_agent,remote_address,expires_at)
		VALUES($1,$2,$3,$4,$5,$6)
	)
	UPDATE occccad.users SET failed_login_count=0,locked_until=NULL,updated_at=now() WHERE id=$1`,
		user.ID, hashToken(token), hashToken(csrf), truncate(userAgent, 500), truncate(remoteAddress, 100), expires)
	if err != nil {
		return Session{}, err
	}
	return Session{User: user, Token: token, CSRFToken: csrf, ExpiresAt: expires}, nil
}

func randomToken() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func hashToken(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func truncate(value string, length int) string {
	if len(value) > length {
		return value[:length]
	}
	return value
}

func (service *Service) Authenticate(ctx context.Context, token string) (User, error) {
	if strings.TrimSpace(token) == "" {
		return User{}, ErrUnauthorized
	}
	var user User
	err := service.database.QueryRow(ctx, `SELECT u.id::text,u.email,u.display_name,u.status,u.platform_role,
		u.must_change_password,u.created_at::text,s.last_seen_at::text
		FROM occccad.user_sessions s JOIN occccad.users u ON u.id=s.user_id
		WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>now() AND u.status='ACTIVE'`,
		hashToken(token)).Scan(&user.ID, &user.Email, &user.DisplayName, &user.Status,
		&user.PlatformRole, &user.MustChangePassword, &user.CreatedAt, &user.LastLoginAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUnauthorized
	}
	if err == nil {
		_, _ = service.database.Exec(ctx, `UPDATE occccad.user_sessions SET last_seen_at=now()
			WHERE token_hash=$1 AND last_seen_at<now()-interval '5 minutes'`, hashToken(token))
	}
	return user, err
}

func (service *Service) ValidateCSRF(ctx context.Context, token, csrf string) error {
	var valid bool
	err := service.database.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM occccad.user_sessions
		WHERE token_hash=$1 AND csrf_hash=$2 AND revoked_at IS NULL AND expires_at>now())`,
		hashToken(token), hashToken(csrf)).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return ErrUnauthorized
	}
	return nil
}

func (service *Service) Logout(ctx context.Context, token string) error {
	_, err := service.database.Exec(ctx, `UPDATE occccad.user_sessions SET revoked_at=now()
		WHERE token_hash=$1 AND revoked_at IS NULL`, hashToken(token))
	return err
}

func (service *Service) ListUsers(ctx context.Context, query, status string) ([]User, error) {
	rows, err := service.database.Query(ctx, `SELECT u.id::text,u.email,u.display_name,u.status,u.platform_role,
		u.must_change_password,u.created_at::text,max(s.last_seen_at)::text
		FROM occccad.users u LEFT JOIN occccad.user_sessions s ON s.user_id=u.id
		WHERE ($1='' OR u.email ILIKE '%'||$1||'%' OR u.display_name ILIKE '%'||$1||'%')
		  AND ($2='' OR u.status=$2)
		GROUP BY u.id ORDER BY u.created_at DESC`, strings.TrimSpace(query), strings.ToUpper(strings.TrimSpace(status)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []User{}
	for rows.Next() {
		var item User
		if err := rows.Scan(&item.ID, &item.Email, &item.DisplayName, &item.Status, &item.PlatformRole,
			&item.MustChangePassword, &item.CreatedAt, &item.LastLoginAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (service *Service) CreateUser(ctx context.Context, actorID string, request CreateUserRequest) (User, error) {
	status := strings.ToUpper(strings.TrimSpace(request.Status))
	if status == "" {
		status = "ACTIVE"
	}
	role := strings.ToUpper(strings.TrimSpace(request.PlatformRole))
	if role == "" {
		role = "MEMBER"
	}
	if status != "ACTIVE" && status != "PENDING" && status != "DISABLED" || role != "ADMIN" && role != "MEMBER" {
		return User{}, fmt.Errorf("%w: unsupported status or platform role", ErrValidation)
	}
	email, err := normalizeEmail(request.Email)
	if err != nil {
		return User{}, err
	}
	name, err := validateDisplayName(request.DisplayName)
	if err != nil {
		return User{}, err
	}
	if err := validatePassword(request.Password); err != nil {
		return User{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(request.Password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}
	var result User
	err = service.database.QueryRow(ctx, `INSERT INTO occccad.users(email,display_name,status,platform_role,
		password_hash,must_change_password,approved_at,approved_by_user_id)
		VALUES($1,$2,$3,$4,$5,true,CASE WHEN $3='ACTIVE' THEN now() END,$6)
		RETURNING id::text,email,display_name,status,platform_role,must_change_password,created_at::text,NULL`,
		email, name, status, role, string(hash), actorID).Scan(&result.ID, &result.Email,
		&result.DisplayName, &result.Status, &result.PlatformRole, &result.MustChangePassword,
		&result.CreatedAt, &result.LastLoginAt)
	if err != nil && strings.Contains(err.Error(), "users_email_idx") {
		return User{}, fmt.Errorf("%w: email is already registered", ErrValidation)
	}
	if err == nil {
		_ = service.audit(ctx, actorID, &result.ID, "CREATE_USER", map[string]any{"status": status, "role": role})
	}
	return result, err
}

func (service *Service) UpdateUser(ctx context.Context, actorID, targetID string, request UpdateUserRequest) (User, error) {
	name, err := validateDisplayName(request.DisplayName)
	if err != nil {
		return User{}, err
	}
	status := strings.ToUpper(strings.TrimSpace(request.Status))
	role := strings.ToUpper(strings.TrimSpace(request.PlatformRole))
	if status != "ACTIVE" && status != "PENDING" && status != "DISABLED" || role != "ADMIN" && role != "MEMBER" {
		return User{}, fmt.Errorf("%w: unsupported status or platform role", ErrValidation)
	}
	if actorID == targetID && (status != "ACTIVE" || role != "ADMIN") {
		return User{}, fmt.Errorf("%w: administrators cannot remove their own active admin access", ErrValidation)
	}
	if err := service.ensureAdminRemains(ctx, targetID, status, role); err != nil {
		return User{}, err
	}
	var result User
	err = service.database.QueryRow(ctx, `UPDATE occccad.users SET display_name=$1,status=$2,platform_role=$3,
		approved_at=CASE WHEN $2='ACTIVE' THEN COALESCE(approved_at,now()) ELSE approved_at END,
		approved_by_user_id=CASE WHEN $2='ACTIVE' THEN $4 ELSE approved_by_user_id END,updated_at=now()
		WHERE id=$5 RETURNING id::text,email,display_name,status,platform_role,must_change_password,created_at::text,NULL`,
		name, status, role, actorID, targetID).Scan(&result.ID, &result.Email, &result.DisplayName,
		&result.Status, &result.PlatformRole, &result.MustChangePassword, &result.CreatedAt, &result.LastLoginAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err == nil {
		if status != "ACTIVE" {
			_, _ = service.database.Exec(ctx, `UPDATE occccad.user_sessions SET revoked_at=now()
				WHERE user_id=$1 AND revoked_at IS NULL`, targetID)
		}
		_ = service.audit(ctx, actorID, &targetID, "UPDATE_USER", map[string]any{"status": status, "role": role})
	}
	return result, err
}

func (service *Service) ensureAdminRemains(ctx context.Context, targetID, status, role string) error {
	var targetIsAdmin bool
	if err := service.database.QueryRow(ctx, `SELECT platform_role='ADMIN' AND status='ACTIVE'
		FROM occccad.users WHERE id=$1`, targetID).Scan(&targetIsAdmin); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if !targetIsAdmin || status == "ACTIVE" && role == "ADMIN" {
		return nil
	}
	var count int
	if err := service.database.QueryRow(ctx, `SELECT count(*) FROM occccad.users
		WHERE status='ACTIVE' AND platform_role='ADMIN'`).Scan(&count); err != nil {
		return err
	}
	if count <= 1 {
		return fmt.Errorf("%w: at least one active administrator is required", ErrValidation)
	}
	return nil
}

func (service *Service) ResetPassword(ctx context.Context, actorID, targetID, password string) error {
	if actorID == targetID {
		return fmt.Errorf("%w: use change password for your own account", ErrValidation)
	}
	if err := validatePassword(password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	command, err := service.database.Exec(ctx, `UPDATE occccad.users SET password_hash=$1,
		must_change_password=true,failed_login_count=0,locked_until=NULL,updated_at=now() WHERE id=$2`,
		string(hash), targetID)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	_, _ = service.database.Exec(ctx, `UPDATE occccad.user_sessions SET revoked_at=now()
		WHERE user_id=$1 AND revoked_at IS NULL`, targetID)
	return service.audit(ctx, actorID, &targetID, "RESET_PASSWORD", nil)
}

func (service *Service) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	var currentHash string
	if err := service.database.QueryRow(ctx, `SELECT password_hash FROM occccad.users WHERE id=$1`, userID).
		Scan(&currentHash); err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(currentPassword)) != nil {
		return ErrUnauthorized
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = service.database.Exec(ctx, `UPDATE occccad.users SET password_hash=$1,
		must_change_password=false,updated_at=now() WHERE id=$2`, string(newHash), userID)
	if err == nil {
		_ = service.audit(ctx, userID, &userID, "CHANGE_PASSWORD", nil)
	}
	return err
}

func (service *Service) audit(ctx context.Context, actorID string, targetID *string, action string, metadata map[string]any) error {
	encoded, _ := json.Marshal(metadata)
	_, err := service.database.Exec(ctx, `INSERT INTO occccad.account_audit_events(
		actor_user_id,target_user_id,action,metadata) VALUES($1,$2,$3,$4)`, actorID, targetID, action, encoded)
	return err
}
