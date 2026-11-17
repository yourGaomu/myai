package core

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	adapterthreadpool "myai/core/adapter/async/threadpool"
	generationadapter "myai/core/adapter/chat/generation"
	outboxjournal "myai/core/adapter/persistence/outbox/journal"
	outboxrepository "myai/core/adapter/persistence/outbox/repository"
	runtimeservice "myai/core/application/runtime/service"
	repository "myai/core/port/repository"
)

func (app *Application) initPersistenceQueue() error {
	if app.store == nil {
		return nil
	}
	metadata, ok := app.store.(repository.SessionMessageMetadataStore)
	if !ok {
		return errors.New("session store does not support durable message metadata")
	}
	workspace, err := canonicalPersistenceWorkspace(app.workspace)
	if err != nil {
		return err
	}
	properties := app.properties.Persistence.Outbox
	path := strings.TrimSpace(properties.Path)
	if path == "" {
		root, err := os.UserConfigDir()
		if err != nil {
			return errors.New("cannot locate private persistence directory")
		}
		digest := sha256.Sum256([]byte(workspace))
		path = filepath.Join(root, "myai", "outbox", hex.EncodeToString(digest[:16])+".db")
	} else if !filepath.IsAbs(path) {
		return errors.New("persistence.outbox.path must be absolute")
	}
	if err := validatePersistencePath(workspace, path); err != nil {
		return err
	}
	scope, err := persistenceScope(app.properties.Mongo.URI, app.properties.Mongo.Database, workspace)
	if err != nil {
		return err
	}
	journal, err := outboxjournal.Open(path, outboxjournal.Options{
		Scope: scope, MaxPending: properties.MaxPending, MaxEntries: properties.MaxEntries,
		MaxBytes: properties.MaxBytes, RetainDone: properties.RetainDone,
	})
	if err != nil {
		return err
	}
	queue, err := generationadapter.NewDurableSessionQueue(
		runtimeservice.AsyncTaskService{Executor: adapterthreadpool.Executor{Pool: app.threadPool}},
		journal,
		outboxrepository.Writer{Messages: app.store, Assets: app.store, Metadata: metadata, Sessions: app.store},
	)
	if err != nil {
		_ = journal.Close()
		return err
	}
	app.outboxJournal = journal
	app.persistenceQueue = queue
	return nil
}

func canonicalPersistenceWorkspace(workspace string) (string, error) {
	if strings.TrimSpace(workspace) == "" {
		workspace = "."
	}
	absolute, err := filepath.Abs(workspace)
	if err != nil {
		return "", errors.New("cannot resolve persistence workspace")
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", errors.New("cannot resolve persistence workspace")
	}
	resolved = filepath.Clean(resolved)
	if runtime.GOOS == "windows" {
		resolved = strings.ToLower(resolved)
	}
	return resolved, nil
}

func validatePersistencePath(workspace, path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("persistence.outbox.path must be absolute")
	}
	// Resolve the existing prefix even when the private journal does not exist yet.
	current := filepath.Clean(path)
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			if runtime.GOOS == "windows" {
				resolved = strings.ToLower(resolved)
			}
			relative, relErr := filepath.Rel(workspace, resolved)
			if relErr == nil && filepath.IsLocal(relative) {
				return errors.New("persistence.outbox.path must be outside the tool workspace")
			}
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) || filepath.Dir(current) == current {
			return errors.New("cannot resolve persistence journal path")
		}
		suffix = append(suffix, filepath.Base(current))
		current = filepath.Dir(current)
	}
}

func persistenceScope(rawURI, database, workspace string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURI))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "mongodb" && parsed.Scheme != "mongodb+srv") {
		return "", errors.New("cannot identify persistence backend")
	}
	if strings.TrimSpace(database) == "" {
		return "", errors.New("persistence database is required")
	}
	// Authentication and query parameters are deliberately absent from the journal.
	identity := strings.ToLower(parsed.Scheme+"://"+parsed.Host) + "\x00" + database + "\x00" + workspace
	digest := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(digest[:]), nil
}
