package access

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const DefaultUserID = "00000000-0000-7000-8000-000000000001"

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrValidation   = errors.New("validation")
	ErrNotFound     = errors.New("not found")
)

type Role string

const (
	RoleViewer Role = "VIEWER"
	RoleEditor Role = "EDITOR"
	RoleOwner  Role = "OWNER"
)

func (role Role) Level() int {
	switch role {
	case RoleOwner:
		return 30
	case RoleEditor:
		return 20
	case RoleViewer:
		return 10
	default:
		return 0
	}
}

type User struct {
	ID                 string `json:"id"`
	Email              string `json:"email"`
	DisplayName        string `json:"displayName"`
	Status             string `json:"status"`
	PlatformRole       string `json:"platformRole"`
	MustChangePassword bool   `json:"mustChangePassword"`
}

type Team struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	OwnerUserID string `json:"ownerUserId"`
	MemberCount int    `json:"memberCount"`
}

type TeamMember struct {
	User
	Role string `json:"role"`
}

type Grant struct {
	ID          string `json:"id"`
	SubjectType string `json:"subjectType"`
	SubjectID   string `json:"subjectId"`
	SubjectName string `json:"subjectName"`
	Role        Role   `json:"role"`
	Inherited   bool   `json:"inherited"`
	SourceName  string `json:"sourceName,omitempty"`
}

type ShareRequest struct {
	SubjectType string `json:"subjectType"`
	SubjectID   string `json:"subjectId"`
	Role        Role   `json:"role"`
}

type AuditEvent struct {
	ID           int64          `json:"id"`
	ActorName    string         `json:"actorName"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resourceType,omitempty"`
	ResourceID   *string        `json:"resourceId,omitempty"`
	RequestID    string         `json:"requestId,omitempty"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    string         `json:"createdAt"`
}

type Service struct{ database *pgxpool.Pool }

func New(database *pgxpool.Pool) *Service { return &Service{database: database} }

type principalContextKey struct{}

func WithPrincipal(ctx context.Context, principal User) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

func Principal(ctx context.Context) (User, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(User)
	return principal, ok
}

func (service *Service) ListUsers(ctx context.Context, query string) ([]User, error) {
	rows, err := service.database.Query(ctx, `
		SELECT id::text,email,display_name,status,platform_role,must_change_password FROM occccad.users
		WHERE status='ACTIVE' AND ($1='' OR email ILIKE '%'||$1||'%' OR display_name ILIKE '%'||$1||'%')
		ORDER BY lower(display_name) LIMIT 50`, strings.TrimSpace(query))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []User{}
	for rows.Next() {
		var item User
		if err := rows.Scan(&item.ID, &item.Email, &item.DisplayName, &item.Status,
			&item.PlatformRole, &item.MustChangePassword); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (service *Service) ListTeams(ctx context.Context, principalID string) ([]Team, error) {
	rows, err := service.database.Query(ctx, `
		SELECT t.id::text,t.name,t.description,t.owner_user_id::text,count(tm.user_id)::int
		FROM occccad.teams t LEFT JOIN occccad.team_members tm ON tm.team_id=t.id
		WHERE t.owner_user_id=$1 OR EXISTS (
			SELECT 1 FROM occccad.team_members mine WHERE mine.team_id=t.id AND mine.user_id=$1)
		GROUP BY t.id ORDER BY lower(t.name)`, principalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Team{}
	for rows.Next() {
		var item Team
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.OwnerUserID, &item.MemberCount); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (service *Service) ListTeamMembers(ctx context.Context, teamID, principalID string) ([]TeamMember, error) {
	if err := service.requireTeamMember(ctx, teamID, principalID); err != nil {
		return nil, err
	}
	rows, err := service.database.Query(ctx, `
		SELECT u.id::text,u.email,u.display_name,u.status,tm.role
		FROM occccad.team_members tm JOIN occccad.users u ON u.id=tm.user_id
		WHERE tm.team_id=$1 ORDER BY lower(u.display_name)`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []TeamMember{}
	for rows.Next() {
		var item TeamMember
		if err := rows.Scan(&item.ID, &item.Email, &item.DisplayName, &item.Status, &item.Role); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (service *Service) requireTeamMember(ctx context.Context, teamID, principalID string) error {
	var allowed bool
	err := service.database.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM occccad.teams t WHERE t.id=$1 AND (t.owner_user_id=$2 OR EXISTS(
			SELECT 1 FROM occccad.team_members tm WHERE tm.team_id=t.id AND tm.user_id=$2)))`,
		teamID, principalID).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

func (service *Service) EffectiveDocumentRole(ctx context.Context, documentID, principalID string) (Role, error) {
	return service.effectiveRole(ctx, `SELECT occccad.role_name(occccad.effective_document_role($1,$2))`, documentID, principalID)
}

func (service *Service) EffectiveFolderRole(ctx context.Context, folderID, principalID string) (Role, error) {
	return service.effectiveRole(ctx, `SELECT occccad.role_name(occccad.effective_folder_role($1,$2))`, folderID, principalID)
}

func (service *Service) effectiveRole(ctx context.Context, statement, resourceID, principalID string) (Role, error) {
	if _, err := uuid.Parse(resourceID); err != nil {
		return "", fmt.Errorf("%w: resource id is invalid", ErrValidation)
	}
	var role Role
	if err := service.database.QueryRow(ctx, statement, resourceID, principalID).Scan(&role); err != nil {
		return "", err
	}
	return role, nil
}

func (service *Service) RequireDocument(ctx context.Context, documentID, principalID string, required Role) (Role, error) {
	role, err := service.EffectiveDocumentRole(ctx, documentID, principalID)
	if err != nil {
		return "", err
	}
	if role.Level() < required.Level() {
		return role, ErrForbidden
	}
	return role, nil
}

func (service *Service) RequireFolder(ctx context.Context, folderID, principalID string, required Role) (Role, error) {
	role, err := service.EffectiveFolderRole(ctx, folderID, principalID)
	if err != nil {
		return "", err
	}
	if role.Level() < required.Level() {
		return role, ErrForbidden
	}
	return role, nil
}

func (service *Service) ListGrants(ctx context.Context, resourceType, resourceID string) ([]Grant, error) {
	resourceType = strings.ToUpper(resourceType)
	if resourceType != "DOCUMENT" && resourceType != "FOLDER" {
		return nil, fmt.Errorf("%w: unsupported resource type", ErrValidation)
	}
	rows, err := service.database.Query(ctx, `
		SELECT g.id::text,CASE WHEN g.user_id IS NOT NULL THEN 'USER' ELSE 'TEAM' END,
		       COALESCE(g.user_id,g.team_id)::text,COALESCE(u.display_name,t.name),g.role
		FROM occccad.resource_grants g
		LEFT JOIN occccad.users u ON u.id=g.user_id LEFT JOIN occccad.teams t ON t.id=g.team_id
		WHERE g.resource_type=$1 AND g.resource_id=$2 ORDER BY lower(COALESCE(u.display_name,t.name))`,
		resourceType, resourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Grant{}
	for rows.Next() {
		var item Grant
		if err := rows.Scan(&item.ID, &item.SubjectType, &item.SubjectID, &item.SubjectName, &item.Role); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (service *Service) UpsertGrant(ctx context.Context, resourceType, resourceID, actorID string, request ShareRequest) (Grant, error) {
	resourceType = strings.ToUpper(resourceType)
	subjectType := strings.ToUpper(strings.TrimSpace(request.SubjectType))
	if (resourceType != "DOCUMENT" && resourceType != "FOLDER") ||
		(subjectType != "USER" && subjectType != "TEAM") ||
		(request.Role != RoleViewer && request.Role != RoleEditor) {
		return Grant{}, fmt.Errorf("%w: share requires a user/team and VIEWER/EDITOR role", ErrValidation)
	}
	if _, err := uuid.Parse(request.SubjectID); err != nil {
		return Grant{}, fmt.Errorf("%w: subject id is invalid", ErrValidation)
	}
	var subjectExists bool
	if subjectType == "USER" {
		if err := service.database.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM occccad.users WHERE id=$1 AND status='ACTIVE')`, request.SubjectID).
			Scan(&subjectExists); err != nil {
			return Grant{}, err
		}
	} else if err := service.database.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM occccad.teams WHERE id=$1)`, request.SubjectID).
		Scan(&subjectExists); err != nil {
		return Grant{}, err
	}
	if !subjectExists {
		return Grant{}, fmt.Errorf("%w: share subject does not exist", ErrValidation)
	}
	var id string
	if subjectType == "USER" {
		err := service.database.QueryRow(ctx, `
			INSERT INTO occccad.resource_grants(resource_type,resource_id,user_id,role,granted_by_user_id)
			VALUES($1,$2,$3,$4,$5)
			ON CONFLICT (resource_type,resource_id,user_id) WHERE user_id IS NOT NULL
			DO UPDATE SET role=excluded.role,granted_by_user_id=excluded.granted_by_user_id,updated_at=now()
			RETURNING id::text`, resourceType, resourceID, request.SubjectID, request.Role, actorID).Scan(&id)
		if err != nil {
			return Grant{}, err
		}
	} else {
		err := service.database.QueryRow(ctx, `
			INSERT INTO occccad.resource_grants(resource_type,resource_id,team_id,role,granted_by_user_id)
			VALUES($1,$2,$3,$4,$5)
			ON CONFLICT (resource_type,resource_id,team_id) WHERE team_id IS NOT NULL
			DO UPDATE SET role=excluded.role,granted_by_user_id=excluded.granted_by_user_id,updated_at=now()
			RETURNING id::text`, resourceType, resourceID, request.SubjectID, request.Role, actorID).Scan(&id)
		if err != nil {
			return Grant{}, err
		}
	}
	grants, err := service.ListGrants(ctx, resourceType, resourceID)
	if err != nil {
		return Grant{}, err
	}
	for _, grant := range grants {
		if grant.ID == id {
			return grant, nil
		}
	}
	return Grant{}, ErrNotFound
}

func (service *Service) DeleteGrant(ctx context.Context, resourceType, resourceID, grantID string) error {
	command, err := service.database.Exec(ctx, `DELETE FROM occccad.resource_grants
		WHERE id=$1 AND resource_type=$2 AND resource_id=$3`, grantID, strings.ToUpper(resourceType), resourceID)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (service *Service) RecordAudit(ctx context.Context, actorID, action, resourceType string,
	resourceID *string, requestID, traceID string, metadata map[string]any) error {
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = service.database.Exec(ctx, `INSERT INTO occccad.access_audit_events(
		actor_user_id,action,resource_type,resource_id,request_id,trace_id,metadata)
		VALUES($1,$2,NULLIF($3,''),$4,NULLIF($5,''),NULLIF($6,''),$7)`,
		actorID, action, strings.ToUpper(resourceType), resourceID, requestID, traceID, encoded)
	return err
}

func (service *Service) ListAudit(ctx context.Context, resourceType, resourceID string, limit int) ([]AuditEvent, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := service.database.Query(ctx, `
		SELECT a.id,COALESCE(u.display_name,'System'),a.action,COALESCE(a.resource_type,''),
		       a.resource_id::text,COALESCE(a.request_id,''),a.metadata,a.created_at::text
		FROM occccad.access_audit_events a LEFT JOIN occccad.users u ON u.id=a.actor_user_id
		WHERE ($1='' OR a.resource_type=$1) AND ($2='' OR a.resource_id=$2::uuid)
		ORDER BY a.id DESC LIMIT $3`, strings.ToUpper(resourceType), resourceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []AuditEvent{}
	for rows.Next() {
		var item AuditEvent
		var metadata []byte
		if err := rows.Scan(&item.ID, &item.ActorName, &item.Action, &item.ResourceType,
			&item.ResourceID, &item.RequestID, &metadata, &item.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(metadata, &item.Metadata)
		result = append(result, item)
	}
	return result, rows.Err()
}
