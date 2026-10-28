package httpapi

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"harbor/internal/providers"
	"harbor/internal/replicate"
	"harbor/internal/store"

	"github.com/go-chi/chi/v5"
)

func (a *API) listDestinations(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.ListDestinations(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list destinations")
		return
	}
	views := make([]destinationView, 0, len(items))
	for _, item := range items {
		view := withFree(item)
		scope := item.AccountID
		if scope == "" {
			scope = item.AccessKeyID
		}
		used, err := a.store.AccountUsage(r.Context(), item.Provider, scope)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not list destinations")
			return
		}
		view.AccountUsedBytes = used
		views = append(views, view)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": views})
}

func (a *API) listProviders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": providers.All()})
}

func (a *API) createDestination(w http.ResponseWriter, r *http.Request) {
	var body destinationBody
	if err := decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	dest, err := normalizeDestination(body, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := checkDestination(r.Context(), dest); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := a.store.CreateDestination(r.Context(), dest)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save destination")
		return
	}
	actor := principalFrom(r)
	a.store.AddAudit(r.Context(), actor.Actor(), "destination.created", created.ID, created.Name)
	writeJSON(w, http.StatusCreated, withFree(created))
}

func (a *API) deleteDestination(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "destinationID")
	current, err := a.store.GetDestination(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "destination not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load destination")
		return
	}
	if err := a.store.DeleteDestination(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete destination")
		return
	}
	actor := principalFrom(r)
	a.store.AddAudit(r.Context(), actor.Actor(), "destination.deleted", id, current.Name)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) testDestination(w http.ResponseWriter, r *http.Request) {
	dest, err := a.store.GetDestination(r.Context(), chi.URLParam(r, "destinationID"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "destination not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load destination")
		return
	}
	if err := checkDestination(r.Context(), dest); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) replicateProject(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustProject(w, r); !ok {
		return
	}
	result, err := replicate.SyncProject(r.Context(), a.store, chi.URLParam(r, "projectID"))
	if errors.Is(err, replicate.ErrNoDestination) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not copy backups")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *API) replicateBackup(w http.ResponseWriter, r *http.Request) {
	backup, ok := a.loadBackup(w, r, "backup:read")
	if !ok {
		return
	}
	if _, err := replicate.SyncBackup(r.Context(), a.store, backup); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "backup not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not copy backup")
		return
	}
	fresh, err := a.store.GetBackup(r.Context(), backup.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load backup")
		return
	}
	writeJSON(w, http.StatusOK, fresh)
}

type destinationBody struct {
	Name            string `json:"name"`
	Provider        string `json:"provider"`
	AccountID       string `json:"account_id"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	Bucket          string `json:"bucket"`
	Jurisdiction    string `json:"jurisdiction"`
	Region          string `json:"region"`
	Endpoint        string `json:"endpoint"`
	KeyPrefix       string `json:"key_prefix"`
	AllowOverFree   bool   `json:"allow_over_free"`
}

type destinationView struct {
	store.Destination
	ProviderName     string `json:"provider_name"`
	FreeStorageBytes int64  `json:"free_storage_bytes"`
	AccountUsedBytes int64  `json:"account_used_bytes"`
}

func withFree(d store.Destination) destinationView {
	d.Secret = ""
	return destinationView{
		Destination:      d,
		ProviderName:     providers.Label(d.Provider),
		FreeStorageBytes: providers.FreeBytes(d.Provider),
	}
}

func normalizeDestination(body destinationBody, creating bool) (store.Destination, error) {
	name := strings.TrimSpace(body.Name)
	if name == "" || len(name) > 80 {
		return store.Destination{}, errors.New("destination name is required")
	}
	access := strings.TrimSpace(body.AccessKeyID)
	if !validAccess(access) {
		return store.Destination{}, errors.New("access key is required")
	}
	secret := strings.TrimSpace(body.SecretAccessKey)
	if creating && !validSecret(secret) {
		return store.Destination{}, errors.New("secret key is required")
	}
	bucket := strings.ToLower(strings.TrimSpace(body.Bucket))
	if !bucketPattern.MatchString(bucket) || strings.Contains(bucket, "..") {
		return store.Destination{}, errors.New("bucket name must be 3 to 63 lowercase letters, numbers, dots, or hyphens")
	}
	prefix := strings.TrimSpace(body.KeyPrefix)
	if prefix == "" {
		prefix = "harbor"
	}
	if !validPrefix(prefix) {
		return store.Destination{}, errors.New("key prefix can only use letters, numbers, and / . _ -")
	}
	accountID := strings.TrimSpace(body.AccountID)
	jurisdiction := body.Jurisdiction
	if body.Provider == store.ProviderCloudflareR2 {
		var hinted string
		accountID, hinted = parseAccount(accountID)
		if strings.TrimSpace(jurisdiction) == "" {
			jurisdiction = hinted
		}
		accountID = strings.ToLower(accountID)
	}
	return providers.Prepare(store.Destination{
		Name: name, Provider: body.Provider, AccountID: accountID,
		AccessKeyID: access, Secret: secret, Bucket: bucket,
		Jurisdiction: jurisdiction, Region: body.Region, Endpoint: body.Endpoint,
		AllowOverFree: body.AllowOverFree, KeyPrefix: prefix,
	})
}

var bucketPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

func validAccess(access string) bool {
	if len(access) < 3 || len(access) > 128 {
		return false
	}
	for _, r := range access {
		if r <= 32 || r > 126 {
			return false
		}
	}
	return true
}

func validSecret(secret string) bool {
	if len(secret) < 8 || len(secret) > 512 {
		return false
	}
	for _, r := range secret {
		if r <= 32 || r > 126 {
			return false
		}
	}
	return true
}

func parseAccount(raw string) (account, jurisdiction string) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
	raw = strings.Trim(raw, "/")
	if cut, _, ok := strings.Cut(raw, "/"); ok {
		raw = cut
	}
	if !strings.Contains(raw, "r2.cloudflarestorage.com") {
		return raw, ""
	}
	parts := strings.Split(raw, ".")
	if len(parts) >= 4 && (parts[1] == "eu" || parts[1] == "fedramp") {
		return parts[0], parts[1]
	}
	if len(parts) > 0 {
		return parts[0], ""
	}
	return raw, ""
}

func validPrefix(prefix string) bool {
	if len(prefix) > 200 || strings.Contains(prefix, "..") || strings.Contains(prefix, "\\") {
		return false
	}
	for _, r := range prefix {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '/' || r == '-' || r == '_' || r == '.':
		default:
			return false
		}
	}
	return !strings.HasPrefix(prefix, "/") && !strings.HasSuffix(prefix, "/")
}

func checkDestination(ctx context.Context, dest store.Destination) error {
	client, err := providers.Open(dest)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return client.Check(ctx)
}
