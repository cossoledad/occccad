package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/occccad/occccad/internal/authn"
)

const (
	sessionCookieName = "occccad_session"
	csrfCookieName    = "occccad_csrf"
)

func publicAPI(path string) bool {
	return path == "/api/health" || path == "/api/auth/login" || path == "/api/auth/register"
}

func (server *Server) setSessionCookies(writer http.ResponseWriter, session authn.Session) {
	http.SetCookie(writer, &http.Cookie{Name: sessionCookieName, Value: session.Token, Path: "/",
		Expires: session.ExpiresAt, MaxAge: int(time.Until(session.ExpiresAt).Seconds()), HttpOnly: true,
		Secure: server.secureCookies, SameSite: http.SameSiteLaxMode})
	http.SetCookie(writer, &http.Cookie{Name: csrfCookieName, Value: session.CSRFToken, Path: "/",
		Expires: session.ExpiresAt, MaxAge: int(time.Until(session.ExpiresAt).Seconds()), HttpOnly: false,
		Secure: server.secureCookies, SameSite: http.SameSiteLaxMode})
}

func (server *Server) clearSessionCookies(writer http.ResponseWriter) {
	for _, name := range []string{sessionCookieName, csrfCookieName} {
		http.SetCookie(writer, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1,
			Expires: time.Unix(1, 0), HttpOnly: name == sessionCookieName,
			Secure: server.secureCookies, SameSite: http.SameSiteLaxMode})
	}
}

func (server *Server) login(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(writer, request, &input) {
		return
	}
	session, err := server.authn.Login(request.Context(), input.Email, input.Password,
		request.UserAgent(), request.RemoteAddr)
	if err != nil {
		writeAuthError(writer, err)
		return
	}
	server.setSessionCookies(writer, session)
	writeJSON(writer, http.StatusOK, map[string]any{"user": session.User})
}

func (server *Server) register(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Email       string `json:"email"`
		DisplayName string `json:"displayName"`
		Password    string `json:"password"`
	}
	if !decodeJSON(writer, request, &input) {
		return
	}
	user, err := server.authn.Register(request.Context(), authn.RegisterRequest{
		Email: input.Email, DisplayName: input.DisplayName, Password: input.Password,
	})
	if err != nil {
		writeAuthError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, map[string]any{
		"user": user, "message": "registration submitted for administrator approval",
	})
}

func (server *Server) logout(writer http.ResponseWriter, request *http.Request) {
	server.openDocuments.CloseAll(principal(request).ID)
	if cookie, err := request.Cookie(sessionCookieName); err == nil {
		_ = server.authn.Logout(request.Context(), cookie.Value)
	}
	server.clearSessionCookies(writer)
	writer.WriteHeader(http.StatusNoContent)
}

func (server *Server) changePassword(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if !decodeJSON(writer, request, &input) {
		return
	}
	if err := server.authn.ChangePassword(request.Context(), principal(request).ID,
		input.CurrentPassword, input.NewPassword); err != nil {
		writeAuthError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"message": "password changed"})
}

func (server *Server) requireAdmin(writer http.ResponseWriter, request *http.Request) bool {
	if principal(request).PlatformRole != "ADMIN" {
		writeError(writer, http.StatusForbidden, "administrator access required")
		return false
	}
	return true
}

func (server *Server) adminListUsers(writer http.ResponseWriter, request *http.Request) {
	if !server.requireAdmin(writer, request) {
		return
	}
	users, err := server.authn.ListUsers(request.Context(), request.URL.Query().Get("q"), request.URL.Query().Get("status"))
	if err != nil {
		writeAuthError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"users": users})
}

func (server *Server) adminCreateUser(writer http.ResponseWriter, request *http.Request) {
	if !server.requireAdmin(writer, request) {
		return
	}
	var input authn.CreateUserRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	user, err := server.authn.CreateUser(request.Context(), principal(request).ID, input)
	if err != nil {
		writeAuthError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, user)
}

func (server *Server) adminUpdateUser(writer http.ResponseWriter, request *http.Request) {
	if !server.requireAdmin(writer, request) {
		return
	}
	var input authn.UpdateUserRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	user, err := server.authn.UpdateUser(request.Context(), principal(request).ID,
		request.PathValue("userID"), input)
	if err != nil {
		writeAuthError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, user)
}

func (server *Server) adminDisableUser(writer http.ResponseWriter, request *http.Request) {
	if !server.requireAdmin(writer, request) {
		return
	}
	users, err := server.authn.ListUsers(request.Context(), "", "")
	if err != nil {
		writeAuthError(writer, err)
		return
	}
	for _, user := range users {
		if user.ID == request.PathValue("userID") {
			_, err = server.authn.UpdateUser(request.Context(), principal(request).ID, user.ID,
				authn.UpdateUserRequest{DisplayName: user.DisplayName, PlatformRole: user.PlatformRole, Status: "DISABLED"})
			if err != nil {
				writeAuthError(writer, err)
				return
			}
			writer.WriteHeader(http.StatusNoContent)
			return
		}
	}
	writeError(writer, http.StatusNotFound, "account not found")
}

func (server *Server) adminResetPassword(writer http.ResponseWriter, request *http.Request) {
	if !server.requireAdmin(writer, request) {
		return
	}
	var input struct {
		Password string `json:"password"`
	}
	if !decodeJSON(writer, request, &input) {
		return
	}
	if err := server.authn.ResetPassword(request.Context(), principal(request).ID,
		request.PathValue("userID"), input.Password); err != nil {
		writeAuthError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"message": "temporary password set"})
}

func (server *Server) adminStats(writer http.ResponseWriter, request *http.Request) {
	if !server.requireAdmin(writer, request) {
		return
	}
	var users, pending, activeSessions, documents int
	err := server.database.QueryRow(request.Context(), `SELECT
		(SELECT count(*) FROM occccad.users),
		(SELECT count(*) FROM occccad.users WHERE status='PENDING'),
		(SELECT count(*) FROM occccad.user_sessions WHERE revoked_at IS NULL AND expires_at>now()),
		(SELECT count(*) FROM occccad.documents WHERE deleted_at IS NULL)`).Scan(&users, &pending, &activeSessions, &documents)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, map[string]int{"users": users, "pending": pending,
		"activeSessions": activeSessions, "documents": documents})
}

func writeAuthError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, authn.ErrUnauthorized):
		writeError(writer, http.StatusUnauthorized, err.Error())
	case errors.Is(err, authn.ErrForbidden):
		writeError(writer, http.StatusForbidden, err.Error())
	case errors.Is(err, authn.ErrValidation):
		writeError(writer, http.StatusBadRequest, strings.TrimPrefix(err.Error(), authn.ErrValidation.Error()+": "))
	case errors.Is(err, authn.ErrNotFound):
		writeError(writer, http.StatusNotFound, err.Error())
	default:
		writeError(writer, http.StatusInternalServerError, err.Error())
	}
}
