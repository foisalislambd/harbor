package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"harbor/internal/config"
	"harbor/internal/store"
	"harbor/internal/webhook"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/crypto/bcrypt"
)

const sessionCookie = "harbor_session"

type API struct {
	cfg   config.Config
	store *store.Store
}

type principal struct {
	User      store.User
	ProjectID string
	KeyID     string
	KeyPrefix string
	Scopes    string
	IsAPIKey  bool
}

func (p principal) Actor() string {
	if p.IsAPIKey {
		return "key:" + p.KeyPrefix
	}
	return "user:" + p.User.Email
}

func New(cfg config.Config, st *store.Store) *API {
	return &API{cfg: cfg, store: st}
}

func (a *API) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Logger)
	if a.cfg.CORSOrigin != "" {
		r.Use(a.cors)
	}

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", a.health)
		r.Get("/setup", a.setupStatus)
		r.Post("/setup", a.setup)
		r.Post("/auth/login", a.login)

		r.Group(func(r chi.Router) {
			r.Use(a.requireUser)
			r.Get("/auth/me", a.me)
			r.Post("/auth/logout", a.logout)
			r.Post("/auth/password", a.changePassword)
			r.Get("/stats", a.stats)
			r.Get("/settings", a.getSettings)
			r.Put("/settings", a.putSettings)
			r.Get("/users", a.listUsers)
			r.Post("/users", a.createUser)
			r.Delete("/users/{userID}", a.deleteUser)
			r.Get("/audit", a.audit)
			r.Get("/projects", a.listProjects)
			r.Post("/projects", a.createProject)
			r.Get("/projects/{projectID}", a.getProject)
			r.Patch("/projects/{projectID}", a.updateProject)
			r.Delete("/projects/{projectID}", a.deleteProject)
			r.Get("/projects/{projectID}/keys", a.listKeys)
			r.Post("/projects/{projectID}/keys", a.createKey)
			r.Delete("/projects/{projectID}/keys/{keyID}", a.deleteKey)
			r.Get("/projects/{projectID}/webhooks", a.listWebhooks)
			r.Post("/projects/{projectID}/webhooks", a.createWebhook)
			r.Delete("/projects/{projectID}/webhooks/{webhookID}", a.deleteWebhook)
			r.Get("/projects/{projectID}/schedules", a.listSchedules)
			r.Post("/projects/{projectID}/schedules", a.createSchedule)
			r.Delete("/projects/{projectID}/schedules/{scheduleID}", a.deleteSchedule)
		})

		r.Group(func(r chi.Router) {
			r.Use(a.requireAny)
			r.Get("/backups", a.listBackups)
			r.Post("/projects/{projectID}/backups", a.uploadBackup)
			r.Post("/push", a.push)
			r.Get("/backups/{backupID}", a.getBackup)
			r.Get("/backups/{backupID}/download", a.downloadBackup)
			r.Delete("/backups/{backupID}", a.deleteBackup)
		})
	})

	if a.cfg.PanelDir != "" {
		r.NotFound(a.spa)
	}
	return r
}

func (a *API) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", a.cfg.CORSOrigin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Vary", "Origin")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *API) spa(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	urlPath := path.Clean("/" + r.URL.Path)
	rel := strings.TrimPrefix(urlPath, "/")
	if rel == "" || rel == "." || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") {
		http.ServeFile(w, r, filepath.Join(a.cfg.PanelDir, "index.html"))
		return
	}
	target := filepath.Join(a.cfg.PanelDir, filepath.FromSlash(rel))
	rooted, err := filepath.Rel(a.cfg.PanelDir, target)
	if err != nil || rooted == ".." || strings.HasPrefix(rooted, ".."+string(filepath.Separator)) {
		http.ServeFile(w, r, filepath.Join(a.cfg.PanelDir, "index.html"))
		return
	}
	info, err := os.Stat(target)
	if err != nil || info.IsDir() {
		http.ServeFile(w, r, filepath.Join(a.cfg.PanelDir, "index.html"))
		return
	}
	http.ServeFile(w, r, target)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decode(r *http.Request, dest any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dest)
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	n, err := a.store.UserCount(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"version":     config.Version,
		"needs_setup": n == 0,
	})
}

func (a *API) setupStatus(w http.ResponseWriter, r *http.Request) {
	n, err := a.store.UserCount(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"needs_setup": n == 0})
}

func (a *API) setup(w http.ResponseWriter, r *http.Request) {
	n, err := a.store.UserCount(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	if n > 0 {
		writeError(w, http.StatusConflict, "this instance is already set up")
		return
	}
	var body struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := validateAccount(body.Name, body.Email, body.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not hash password")
		return
	}
	user, err := a.store.CreateOwner(r.Context(), body.Email, body.Name, string(hash))
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "this instance is already set up")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not create owner")
		return
	}
	_ = a.store.SetSetting(r.Context(), "instance_name", "Harbor")
	a.store.AddAudit(r.Context(), "user:"+user.Email, "instance.setup", user.ID, "")
	a.startSession(w, r, user)
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	user, err := a.store.UserByEmail(r.Context(), body.Email)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(body.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	a.store.AddAudit(r.Context(), "user:"+user.Email, "auth.login", user.ID, "")
	a.startSession(w, r, user)
}

func (a *API) startSession(w http.ResponseWriter, r *http.Request, user store.User) {
	raw, expires, err := a.store.CreateSession(r.Context(), user.ID, a.cfg.SessionTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start session")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    raw,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
		Secure:   a.cfg.CookieSecure,
	})
	user.Password = ""
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	p := principalFrom(r)
	writeJSON(w, http.StatusOK, map[string]any{"user": p.User})
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = a.store.DeleteSession(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1, Expires: time.Unix(0, 0)})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) changePassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(body.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	p := principalFrom(r)
	user, err := a.store.UserByID(r.Context(), p.User.ID)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(body.CurrentPassword)) != nil {
		writeError(w, http.StatusUnauthorized, "current password is wrong")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not hash password")
		return
	}
	if err := a.store.UpdatePassword(r.Context(), user.ID, string(hash)); err != nil {
		writeError(w, http.StatusInternalServerError, "could not update password")
		return
	}
	a.store.AddAudit(r.Context(), p.Actor(), "auth.password", user.ID, "")
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) stats(w http.ResponseWriter, r *http.Request) {
	st, err := a.store.Stats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load stats")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (a *API) getSettings(w http.ResponseWriter, r *http.Request) {
	name, err := a.store.GetSetting(r.Context(), "instance_name", "Harbor")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load settings")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"instance_name":    name,
		"version":          config.Version,
		"data_dir":         a.cfg.DataDir,
		"max_upload_bytes": a.cfg.MaxUpload,
	})
}

func (a *API) putSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		InstanceName string `json:"instance_name"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	name := strings.TrimSpace(body.InstanceName)
	if name == "" || len(name) > 80 {
		writeError(w, http.StatusBadRequest, "instance name is required")
		return
	}
	if err := a.store.SetSetting(r.Context(), "instance_name", name); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save settings")
		return
	}
	p := principalFrom(r)
	a.store.AddAudit(r.Context(), p.Actor(), "settings.updated", "instance_name", name)
	a.getSettings(w, r)
}

func (a *API) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := a.store.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list users")
		return
	}
	if users == nil {
		users = []store.User{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": users})
}

func (a *API) createUser(w http.ResponseWriter, r *http.Request) {
	if principalFrom(r).User.Role != "owner" {
		writeError(w, http.StatusForbidden, "only the owner can manage users")
		return
	}
	var body struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := validateAccount(body.Name, body.Email, body.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not hash password")
		return
	}
	user, err := a.store.CreateUser(r.Context(), body.Email, body.Name, string(hash), "operator")
	if err != nil {
		writeError(w, http.StatusConflict, "a user with that email already exists")
		return
	}
	user.Password = ""
	p := principalFrom(r)
	a.store.AddAudit(r.Context(), p.Actor(), "user.created", user.ID, user.Email)
	writeJSON(w, http.StatusCreated, user)
}

func (a *API) deleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "userID")
	p := principalFrom(r)
	if p.User.Role != "owner" {
		writeError(w, http.StatusForbidden, "only the owner can manage users")
		return
	}
	if id == p.User.ID {
		writeError(w, http.StatusBadRequest, "you cannot delete your own account")
		return
	}
	target, err := a.store.UserByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err := a.store.DeleteUser(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	a.store.AddAudit(r.Context(), p.Actor(), "user.deleted", id, target.Email)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) audit(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := a.store.ListAudit(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load activity")
		return
	}
	if items == nil {
		items = []store.Audit{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

type projectBody struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	Status         string `json:"status"`
	RetentionCount int    `json:"retention_count"`
	RetentionDays  int    `json:"retention_days"`
	QuotaBytes     int64  `json:"quota_bytes"`
}

func (a *API) listProjects(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.ListProjects(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list projects")
		return
	}
	if items == nil {
		items = []store.Project{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *API) createProject(w http.ResponseWriter, r *http.Request) {
	var body projectBody
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := validateProject(body, false); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p, err := a.store.CreateProject(r.Context(), body.Name, body.Description, body.RetentionCount, body.RetentionDays, body.QuotaBytes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create project")
		return
	}
	actor := principalFrom(r)
	a.store.AddAudit(r.Context(), actor.Actor(), "project.created", p.ID, p.Name)
	writeJSON(w, http.StatusCreated, p)
}

func (a *API) getProject(w http.ResponseWriter, r *http.Request) {
	p, err := a.store.GetProject(r.Context(), chi.URLParam(r, "projectID"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load project")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (a *API) updateProject(w http.ResponseWriter, r *http.Request) {
	current, err := a.store.GetProject(r.Context(), chi.URLParam(r, "projectID"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load project")
		return
	}
	var body projectBody
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(body.Status) == "" {
		body.Status = current.Status
	}
	if err := validateProject(body, true); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	current.Name = body.Name
	current.Description = body.Description
	current.Status = body.Status
	current.RetentionCount = body.RetentionCount
	current.RetentionDays = body.RetentionDays
	current.QuotaBytes = body.QuotaBytes
	updated, err := a.store.UpdateProject(r.Context(), current)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update project")
		return
	}
	removed, err := a.store.EnforceRetention(r.Context(), updated.ID)
	if err != nil {
		log.Printf("retention: %v", err)
	}
	for _, b := range removed {
		a.store.AddAudit(r.Context(), "system:retention", "backup.deleted", b.ID, b.Name)
	}
	actor := principalFrom(r)
	a.store.AddAudit(r.Context(), actor.Actor(), "project.updated", updated.ID, updated.Name)
	updated, _ = a.store.GetProject(r.Context(), updated.ID)
	writeJSON(w, http.StatusOK, updated)
}

func (a *API) deleteProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "projectID")
	p, err := a.store.GetProject(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err := a.store.DeleteProject(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete project")
		return
	}
	actor := principalFrom(r)
	a.store.AddAudit(r.Context(), actor.Actor(), "project.deleted", id, p.Name)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) listKeys(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustProject(w, r); !ok {
		return
	}
	items, err := a.store.ListAPIKeys(r.Context(), chi.URLParam(r, "projectID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list keys")
		return
	}
	if items == nil {
		items = []store.APIKey{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *API) createKey(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustProject(w, r); !ok {
		return
	}
	var body struct {
		Name   string `json:"name"`
		Scopes string `json:"scopes"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, "key name is required")
		return
	}
	scopes := strings.TrimSpace(body.Scopes)
	if scopes == "" {
		scopes = "backup:read,backup:write,backup:delete"
	}
	if !validScopes(scopes) {
		writeError(w, http.StatusBadRequest, "scopes must be backup:read, backup:write, backup:delete")
		return
	}
	key, err := a.store.CreateAPIKey(r.Context(), chi.URLParam(r, "projectID"), body.Name, scopes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create key")
		return
	}
	actor := principalFrom(r)
	a.store.AddAudit(r.Context(), actor.Actor(), "key.created", key.ID, key.Prefix)
	writeJSON(w, http.StatusCreated, key)
}

func (a *API) deleteKey(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustProject(w, r); !ok {
		return
	}
	projectID := chi.URLParam(r, "projectID")
	keyID := chi.URLParam(r, "keyID")
	if err := a.store.DeleteAPIKey(r.Context(), projectID, keyID); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "key not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not revoke key")
		return
	}
	actor := principalFrom(r)
	a.store.AddAudit(r.Context(), actor.Actor(), "key.revoked", keyID, projectID)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) listWebhooks(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustProject(w, r); !ok {
		return
	}
	items, err := a.store.ListWebhooks(r.Context(), chi.URLParam(r, "projectID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list webhooks")
		return
	}
	if items == nil {
		items = []store.Webhook{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *API) createWebhook(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustProject(w, r); !ok {
		return
	}
	var body struct {
		URL    string `json:"url"`
		Events string `json:"events"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !strings.HasPrefix(body.URL, "https://") && !strings.HasPrefix(body.URL, "http://") {
		writeError(w, http.StatusBadRequest, "webhook url must start with http:// or https://")
		return
	}
	events := strings.TrimSpace(body.Events)
	if events == "" {
		events = "backup.created,backup.deleted"
	}
	if !validEvents(events) {
		writeError(w, http.StatusBadRequest, "events must be backup.created and/or backup.deleted")
		return
	}
	wh, err := a.store.CreateWebhook(r.Context(), chi.URLParam(r, "projectID"), body.URL, events)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create webhook")
		return
	}
	actor := principalFrom(r)
	a.store.AddAudit(r.Context(), actor.Actor(), "webhook.created", wh.ID, wh.URL)
	writeJSON(w, http.StatusCreated, wh)
}

func (a *API) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustProject(w, r); !ok {
		return
	}
	if err := a.store.DeleteWebhook(r.Context(), chi.URLParam(r, "projectID"), chi.URLParam(r, "webhookID")); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "webhook not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete webhook")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) listSchedules(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustProject(w, r); !ok {
		return
	}
	items, err := a.store.ListSchedules(r.Context(), chi.URLParam(r, "projectID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list schedules")
		return
	}
	if items == nil {
		items = []store.Schedule{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *API) createSchedule(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustProject(w, r); !ok {
		return
	}
	var body struct {
		Name          string `json:"name"`
		IntervalHours int    `json:"interval_hours"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, "schedule name is required")
		return
	}
	if body.IntervalHours < 1 || body.IntervalHours > 24*365 {
		writeError(w, http.StatusBadRequest, "interval must be between 1 and 8760 hours")
		return
	}
	sc, err := a.store.CreateSchedule(r.Context(), chi.URLParam(r, "projectID"), body.Name, body.IntervalHours)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create schedule")
		return
	}
	writeJSON(w, http.StatusCreated, sc)
}

func (a *API) deleteSchedule(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustProject(w, r); !ok {
		return
	}
	if err := a.store.DeleteSchedule(r.Context(), chi.URLParam(r, "projectID"), chi.URLParam(r, "scheduleID")); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "schedule not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete schedule")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) listBackups(w http.ResponseWriter, r *http.Request) {
	p := principalFrom(r)
	f := store.BackupFilter{
		ProjectID: r.URL.Query().Get("project_id"),
		Query:     r.URL.Query().Get("q"),
	}
	f.Limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	f.Offset, _ = strconv.Atoi(r.URL.Query().Get("offset"))
	if p.IsAPIKey {
		if !hasScope(p.Scopes, "backup:read") {
			writeError(w, http.StatusForbidden, "key is missing backup:read")
			return
		}
		f.ProjectID = p.ProjectID
	}
	items, total, err := a.store.ListBackups(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list backups")
		return
	}
	if items == nil {
		items = []store.Backup{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (a *API) push(w http.ResponseWriter, r *http.Request) {
	p := principalFrom(r)
	if !p.IsAPIKey {
		writeError(w, http.StatusUnauthorized, "push requires a project API key")
		return
	}
	a.ingest(w, r, p.ProjectID)
}

func (a *API) uploadBackup(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	if !a.allowProject(w, r, projectID, "backup:write") {
		return
	}
	a.ingest(w, r, projectID)
}

func (a *API) ingest(w http.ResponseWriter, r *http.Request, projectID string) {
	p := principalFrom(r)
	if p.IsAPIKey && !hasScope(p.Scopes, "backup:write") {
		writeError(w, http.StatusForbidden, "key is missing backup:write")
		return
	}
	project, err := a.store.GetProject(r.Context(), projectID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load project")
		return
	}
	if project.Status != "active" {
		writeError(w, http.StatusConflict, "project is paused")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, a.cfg.MaxUpload+1<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "upload exceeds the size limit")
			return
		}
		writeError(w, http.StatusBadRequest, "expected multipart form with a file field")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file field is required")
		return
	}
	defer file.Close()
	filename := cleanFilename(header.Filename)
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		name = filename
	}
	kind := strings.TrimSpace(r.FormValue("kind"))
	if kind == "" {
		kind = "archive"
	}
	source := strings.TrimSpace(r.FormValue("source"))
	note := strings.TrimSpace(r.FormValue("note"))
	tags := strings.TrimSpace(r.FormValue("tags"))
	if !validKind(kind) {
		writeError(w, http.StatusBadRequest, "kind must be archive, database, files, or other")
		return
	}
	if len(name) > 200 || len(source) > 200 || len(note) > 2000 || len(tags) > 500 {
		writeError(w, http.StatusBadRequest, "name, source, note, or tags is too long")
		return
	}

	tmp, err := os.CreateTemp(a.store.TmpDir(), "upload-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not store upload")
		return
	}
	tmpName := tmp.Name()
	hasher := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(tmp, hasher), file)
	closeErr := tmp.Close()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if copyErr != nil || closeErr != nil {
		var maxErr *http.MaxBytesError
		if errors.As(copyErr, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "upload exceeds the size limit")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not store upload")
		return
	}
	if n == 0 {
		writeError(w, http.StatusBadRequest, "file is empty")
		return
	}
	if n > a.cfg.MaxUpload {
		writeError(w, http.StatusRequestEntityTooLarge, "upload exceeds the size limit")
		return
	}
	sum := hex.EncodeToString(hasher.Sum(nil))
	dest, err := a.store.BlobPath(sum)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not store upload")
		return
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "could not store upload")
		return
	}
	_, statErr := os.Stat(dest)
	switch {
	case errors.Is(statErr, os.ErrNotExist):
		if err := os.Rename(tmpName, dest); err != nil {
			if _, err2 := os.Stat(dest); err2 != nil {
				writeError(w, http.StatusInternalServerError, "could not store upload")
				return
			}
		} else {
			cleanup = false
		}
	case statErr != nil:
		writeError(w, http.StatusInternalServerError, "could not store upload")
		return
	}
	ctype := header.Header.Get("Content-Type")
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	if mediatype, _, err := mime.ParseMediaType(ctype); err == nil {
		ctype = mediatype
	}
	backup, err := a.store.CreateBackup(r.Context(), store.Backup{
		ProjectID:   project.ID,
		Name:        name,
		Filename:    filename,
		Kind:        kind,
		Source:      source,
		Note:        note,
		Tags:        tags,
		SizeBytes:   n,
		Checksum:    sum,
		ContentType: ctype,
	})
	if err != nil {
		a.store.ReleaseUnused(r.Context(), sum)
		switch {
		case errors.Is(err, store.ErrQuota):
			writeError(w, http.StatusRequestEntityTooLarge, "project storage quota exceeded")
		case errors.Is(err, store.ErrPaused):
			writeError(w, http.StatusConflict, "project is paused")
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusNotFound, "project not found")
		default:
			writeError(w, http.StatusInternalServerError, "could not record backup")
		}
		return
	}
	backup.ProjectName = project.Name
	removed, err := a.store.EnforceRetention(r.Context(), project.ID)
	if err != nil {
		log.Printf("retention: %v", err)
	}
	a.store.AddAudit(r.Context(), p.Actor(), "backup.created", backup.ID, backup.Name)
	for _, old := range removed {
		a.store.AddAudit(r.Context(), "system:retention", "backup.deleted", old.ID, old.Name)
	}
	a.emit(r, project.ID, "backup.created", backup)
	for _, old := range removed {
		a.emit(r, project.ID, "backup.deleted", old)
	}
	fresh, err := a.store.GetBackup(r.Context(), backup.ID)
	if err == nil {
		backup = fresh
	}
	writeJSON(w, http.StatusCreated, backup)
}

func (a *API) emit(r *http.Request, projectID, event string, data any) {
	hooks, err := a.store.ListWebhooks(r.Context(), projectID)
	if err != nil || len(hooks) == 0 {
		return
	}
	targets := make([]webhook.Target, 0, len(hooks))
	for _, h := range hooks {
		targets = append(targets, webhook.Target{URL: h.URL, Secret: h.Secret, Events: h.Events})
	}
	webhook.Dispatch(targets, event, data)
}

func (a *API) getBackup(w http.ResponseWriter, r *http.Request) {
	b, ok := a.loadBackup(w, r, "backup:read")
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (a *API) downloadBackup(w http.ResponseWriter, r *http.Request) {
	b, ok := a.loadBackup(w, r, "backup:read")
	if !ok {
		return
	}
	path, err := a.store.BlobPath(b.Checksum)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "backup file is missing")
		return
	}
	f, err := os.Open(path)
	if err != nil {
		writeError(w, http.StatusNotFound, "backup file is missing")
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", b.ContentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": b.Filename}))
	w.Header().Set("X-Checksum-Sha256", b.Checksum)
	http.ServeContent(w, r, b.Filename, time.Time{}, f)
}

func (a *API) deleteBackup(w http.ResponseWriter, r *http.Request) {
	b, ok := a.loadBackup(w, r, "backup:delete")
	if !ok {
		return
	}
	if _, err := a.store.DeleteBackup(r.Context(), b.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete backup")
		return
	}
	p := principalFrom(r)
	a.store.AddAudit(r.Context(), p.Actor(), "backup.deleted", b.ID, b.Name)
	a.emit(r, b.ProjectID, "backup.deleted", b)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) loadBackup(w http.ResponseWriter, r *http.Request, scope string) (store.Backup, bool) {
	b, err := a.store.GetBackup(r.Context(), chi.URLParam(r, "backupID"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "backup not found")
		return store.Backup{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load backup")
		return store.Backup{}, false
	}
	if !a.allowProject(w, r, b.ProjectID, scope) {
		return store.Backup{}, false
	}
	return b, true
}

func (a *API) mustProject(w http.ResponseWriter, r *http.Request) (store.Project, bool) {
	p, err := a.store.GetProject(r.Context(), chi.URLParam(r, "projectID"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "project not found")
		return store.Project{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load project")
		return store.Project{}, false
	}
	return p, true
}

func (a *API) allowProject(w http.ResponseWriter, r *http.Request, projectID, scope string) bool {
	p := principalFrom(r)
	if p.IsAPIKey {
		if p.ProjectID != projectID {
			writeError(w, http.StatusNotFound, "backup not found")
			return false
		}
		if !hasScope(p.Scopes, scope) {
			writeError(w, http.StatusForbidden, "key is missing "+scope)
			return false
		}
	}
	return true
}

func (a *API) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := a.authenticate(r)
		if !ok || p.IsAPIKey {
			writeError(w, http.StatusUnauthorized, "sign in required")
			return
		}
		next.ServeHTTP(w, r.WithContext(withPrincipal(r.Context(), p)))
	})
}

func (a *API) requireAny(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := a.authenticate(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		if p.IsAPIKey {
			a.store.TouchAPIKey(r.Context(), p.KeyID)
		}
		next.ServeHTTP(w, r.WithContext(withPrincipal(r.Context(), p)))
	})
}

func (a *API) authenticate(r *http.Request) (principal, bool) {
	raw := bearer(r)
	if raw == "" {
		if c, err := r.Cookie(sessionCookie); err == nil {
			raw = c.Value
		}
	}
	if raw == "" {
		return principal{}, false
	}
	if strings.HasPrefix(raw, "hb_") {
		key, err := a.store.APIKeyByToken(r.Context(), raw)
		if err != nil {
			return principal{}, false
		}
		return principal{ProjectID: key.ProjectID, KeyID: key.ID, KeyPrefix: key.Prefix, Scopes: key.Scopes, IsAPIKey: true}, true
	}
	user, err := a.store.UserBySession(r.Context(), raw)
	if err != nil {
		return principal{}, false
	}
	return principal{User: user}, true
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) < 7 || !strings.EqualFold(h[:7], "bearer ") {
		return ""
	}
	return strings.TrimSpace(h[7:])
}

type ctxKey struct{}

func withPrincipal(ctx context.Context, p principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

func principalFrom(r *http.Request) principal {
	p, _ := r.Context().Value(ctxKey{}).(principal)
	return p
}

func validateAccount(name, email, password string) error {
	if strings.TrimSpace(name) == "" || len(strings.TrimSpace(name)) > 80 {
		return errors.New("name is required")
	}
	email = strings.TrimSpace(email)
	if !strings.Contains(email, "@") || strings.HasPrefix(email, "@") || strings.HasSuffix(email, "@") || len(email) > 200 {
		return errors.New("a valid email is required")
	}
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	return nil
}

func validateProject(body projectBody, updating bool) error {
	name := strings.TrimSpace(body.Name)
	if name == "" || len(name) > 120 {
		return errors.New("project name is required")
	}
	if len(body.Description) > 2000 {
		return errors.New("description is too long")
	}
	if body.RetentionCount < 0 || body.RetentionDays < 0 || body.QuotaBytes < 0 {
		return errors.New("retention and quota cannot be negative")
	}
	if updating && body.Status != "active" && body.Status != "paused" {
		return errors.New("status must be active or paused")
	}
	return nil
}

func validScopes(scopes string) bool {
	allowed := map[string]bool{"backup:read": true, "backup:write": true, "backup:delete": true}
	seen := map[string]bool{}
	for _, part := range strings.Split(scopes, ",") {
		part = strings.TrimSpace(part)
		if !allowed[part] || seen[part] {
			return false
		}
		seen[part] = true
	}
	return len(seen) > 0
}

func hasScope(scopes, want string) bool {
	for _, part := range strings.Split(scopes, ",") {
		if strings.TrimSpace(part) == want {
			return true
		}
	}
	return false
}

func validEvents(events string) bool {
	allowed := map[string]bool{"backup.created": true, "backup.deleted": true}
	seen := map[string]bool{}
	for _, part := range strings.Split(events, ",") {
		part = strings.TrimSpace(part)
		if !allowed[part] || seen[part] {
			return false
		}
		seen[part] = true
	}
	return len(seen) > 0
}

func validKind(kind string) bool {
	switch kind {
	case "archive", "database", "files", "other":
		return true
	default:
		return false
	}
}

func cleanFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "." || name == "" || name == string(filepath.Separator) {
		return "backup.bin"
	}
	var b strings.Builder
	for _, r := range name {
		if r < 32 || r == '"' || r == '/' || r == '\\' {
			b.WriteByte('_')
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	if len(out) > 180 {
		out = out[:180]
	}
	if strings.Trim(out, "._ ") == "" {
		return "backup.bin"
	}
	return out
}
