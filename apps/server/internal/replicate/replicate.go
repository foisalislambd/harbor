// Package replicate copies a backup that already lives on this machine to the
// destination selected for its project, and removes that copy when nothing
// here still needs it.
package replicate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"

	"harbor/internal/providers"
	"harbor/internal/store"
)

var openClient = providers.Open

var storageLimit = func(d store.Destination) int64 {
	if d.AllowOverFree {
		return 0
	}
	return providers.FreeBytes(d.Provider)
}

var copyMu sync.Mutex

// ErrNoDestination means the project keeps backups on this machine only.
var ErrNoDestination = errors.New("choose a destination for this project first")

type Result struct {
	Copied  int `json:"copied"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
	Already int `json:"already"`
}

// ObjectKey is the content-addressed name used for every provider.
func ObjectKey(prefix, checksum string) string {
	prefix = trimSlashes(prefix)
	if prefix == "" {
		return "blobs/" + checksum
	}
	return prefix + "/blobs/" + checksum
}

func trimSlashes(s string) string {
	for len(s) > 0 && s[0] == '/' {
		s = s[1:]
	}
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

// SyncBackup copies one backup to the project's destination. An empty status
// means the project has no destination. A Cloudflare error is stored on the
// replica and returned as "failed" so the local backup still stands.
func SyncBackup(ctx context.Context, st *store.Store, backup store.Backup) (string, error) {
	copyMu.Lock()
	defer copyMu.Unlock()
	project, err := st.GetProject(ctx, backup.ProjectID)
	if err != nil {
		return "", err
	}
	if project.DestinationID == "" {
		return "", nil
	}
	dest, err := st.GetDestination(ctx, project.DestinationID)
	if err != nil {
		return "", err
	}
	key := ObjectKey(dest.KeyPrefix, backup.Checksum)
	existing, err := st.GetReplica(ctx, backup.ID, dest.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return "", err
	}
	if err == nil && existing.Status == storeStatusSynced && existing.ObjectKey == key {
		return "already", nil
	}
	present, err := st.ObjectSynced(ctx, dest.ID, key)
	if err != nil {
		return "", err
	}
	shared := false
	if !present {
		shared, err = st.RemoteObjectStored(ctx, dest, key)
		if err != nil {
			return "", err
		}
	}
	if !present && !shared {
		limit := storageLimit(dest)
		if limit > 0 {
			scope := dest.AccountID
			if scope == "" {
				scope = dest.AccessKeyID
			}
			used, err := st.AccountUsage(ctx, dest.Provider, scope)
			if err != nil {
				return "", err
			}
			if used+backup.SizeBytes > limit {
				msg := fmt.Sprintf("%s free storage holds %s. This backup stays on this machine.", providers.Label(dest.Provider), formatBytes(limit))
				if err := st.UpsertReplica(ctx, backup.ID, store.Replica{
					DestinationID: dest.ID, ObjectKey: key, Status: "skipped", Error: msg, SizeBytes: backup.SizeBytes,
				}); err != nil {
					return "", err
				}
				return "skipped", nil
			}
		}
		path, err := st.BlobPath(backup.Checksum)
		if err != nil {
			return "", err
		}
		client, err := openClient(dest)
		if err != nil {
			if saveErr := saveFailure(ctx, st, backup, dest.ID, key, err); saveErr != nil {
				return "", saveErr
			}
			return "failed", nil
		}
		if err := client.PutFile(ctx, key, path, backup.ContentType, backup.Checksum, backup.SizeBytes); err != nil {
			if saveErr := saveFailure(ctx, st, backup, dest.ID, key, err); saveErr != nil {
				return "", saveErr
			}
			return "failed", nil
		}
	}
	if err := st.UpsertReplica(ctx, backup.ID, store.Replica{
		DestinationID: dest.ID, ObjectKey: key, Status: storeStatusSynced, SizeBytes: backup.SizeBytes,
	}); err != nil {
		return "", err
	}
	return "copied", nil
}

const storeStatusSynced = "synced"

func saveFailure(ctx context.Context, st *store.Store, backup store.Backup, destinationID, key string, cause error) error {
	msg := cause.Error()
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return st.UpsertReplica(ctx, backup.ID, store.Replica{
		DestinationID: destinationID, ObjectKey: key, Status: "failed", Error: msg, SizeBytes: backup.SizeBytes,
	})
}

// SyncProject copies every backup already stored for the project.
func SyncProject(ctx context.Context, st *store.Store, projectID string) (Result, error) {
	project, err := st.GetProject(ctx, projectID)
	if err != nil {
		return Result{}, err
	}
	if project.DestinationID == "" {
		return Result{}, ErrNoDestination
	}
	backups, err := st.ListProjectBackups(ctx, projectID)
	if err != nil {
		return Result{}, err
	}
	var result Result
	for _, backup := range backups {
		status, err := SyncBackup(ctx, st, backup)
		if err != nil {
			return result, err
		}
		switch status {
		case "copied":
			result.Copied++
		case "failed":
			result.Failed++
		case "skipped":
			result.Skipped++
		case "already":
			result.Already++
		}
	}
	return result, nil
}

// FlushDeletes removes objects that no remaining backup references.
func FlushDeletes(ctx context.Context, st *store.Store) {
	copyMu.Lock()
	defer copyMu.Unlock()
	items, err := st.ListRemoteDeletes(ctx)
	if err != nil {
		log.Printf("remote delete: %v", err)
		return
	}
	for _, item := range items {
		dest, err := st.GetDestination(ctx, item.DestinationID)
		if errors.Is(err, store.ErrNotFound) {
			_ = st.FinishRemoteDelete(ctx, item.DestinationID, item.ObjectKey)
			continue
		}
		if err != nil {
			log.Printf("remote delete: %v", err)
			continue
		}
		client, err := openClient(dest)
		if err != nil {
			log.Printf("remote delete: %v", err)
			continue
		}
		if err := client.Delete(ctx, item.ObjectKey); err != nil {
			log.Printf("remote delete %s: %v", item.ObjectKey, err)
			continue
		}
		if err := st.FinishRemoteDelete(ctx, item.DestinationID, item.ObjectKey); err != nil {
			log.Printf("remote delete: %v", err)
		}
	}
}

// OpenRemote reads a synced copy when the local file is gone.
func OpenRemote(ctx context.Context, st *store.Store, backup store.Backup) (io.ReadCloser, error) {
	if len(backup.Copies) == 0 {
		fresh, err := st.GetBackup(ctx, backup.ID)
		if err != nil {
			return nil, err
		}
		backup = fresh
	}
	var last error
	for _, copy := range backup.Copies {
		if copy.Status != storeStatusSynced {
			continue
		}
		replica, err := st.GetReplica(ctx, backup.ID, copy.DestinationID)
		if err != nil {
			last = err
			continue
		}
		dest, err := st.GetDestination(ctx, copy.DestinationID)
		if err != nil {
			last = err
			continue
		}
		client, err := openClient(dest)
		if err != nil {
			last = err
			continue
		}
		body, err := client.Open(ctx, replica.ObjectKey)
		if err != nil {
			last = err
			continue
		}
		return body, nil
	}
	if last != nil {
		return nil, last
	}
	return nil, errors.New("backup file is missing")
}

func formatBytes(n int64) string {
	if n > 0 && n%1_000_000_000 == 0 {
		return fmt.Sprintf("%d GB", n/1_000_000_000)
	}
	return fmt.Sprintf("%d bytes", n)
}
