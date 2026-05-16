package main

import (
	"log"
	"os"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	domainsession "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
	memorypersistence "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/memory"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/session"
)

type sessionRuntime struct {
	SessionRepo   orchestrator.SessionRepository
	CentralMemory *domainsession.CentralMemory
	MemoryStore   *memorypersistence.FileStore
}

func buildSessionRuntime(cfg *config.Config) sessionRuntime {
	sessionRepo := session.NewJSONSessionRepository(cfg.Session.StorageDir)
	centralMemory := domainsession.NewCentralMemory()
	if err := os.MkdirAll(cfg.Session.StorageDir, 0755); err != nil {
		log.Fatalf("Failed to create session directory: %v", err)
	}
	memStore := memorypersistence.NewFileStore(cfg.WorkspaceDir)
	log.Printf("MemoryStore initialized (workspace: %s)", cfg.WorkspaceDir)
	return sessionRuntime{
		SessionRepo:   sessionRepo,
		CentralMemory: centralMemory,
		MemoryStore:   memStore,
	}
}
