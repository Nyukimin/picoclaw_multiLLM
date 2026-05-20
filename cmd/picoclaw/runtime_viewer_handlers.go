package main

import (
	"log"
	"path/filepath"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/viewer"
	conversationpersistence "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/conversation"
	executionpersistence "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/execution"
)

func buildViewerRuntimeHandlers(
	cfg *config.Config,
	deps *Dependencies,
	l1Store *conversationpersistence.L1SQLiteStore,
	realMgr *conversationpersistence.RealConversationManager,
	reportPath string,
) {
	if l1Store == nil {
		deps.viewerMemoryLayers = viewer.HandleMemoryLayers(nil, nil)
		deps.viewerSourceRegistry = viewer.HandleSourceRegistry(nil)
	}
	if l1Store != nil {
		deps.viewerMemorySnapshot = viewer.HandleMemorySnapshot(l1Store)
		deps.viewerMemoryLayers = viewer.HandleMemoryLayers(l1Store, realMgr)
		deps.viewerMemoryEvents = viewer.HandleMemoryEvents(l1Store)
		deps.viewerMemoryState = viewer.HandleMemoryState(l1Store)
		deps.viewerMemoryPromote = viewer.HandleMemoryPromote(l1Store)
		deps.viewerRecallTraces = viewer.HandleRecallTraces(l1Store)
		deps.viewerSourceRegistry = viewer.HandleSourceRegistry(l1Store)
	}

	hub := viewer.NewEventHub(200)
	deps.eventHub = hub
	if cfg.ViewerLog.Enabled {
		eventLogPath := cfg.ViewerLog.Path
		if eventLogStore, err := viewer.NewEventLogStore(eventLogPath); err != nil {
			log.Printf("WARN: viewer event log disabled: %v", err)
		} else {
			deps.eventLogStore = eventLogStore
			log.Printf("Viewer event log enabled: %s", eventLogPath)
			gcPath := filepath.Join(filepath.Dir(eventLogPath), "orchestrator_event_gc.jsonl")
			if gcSvc, err := viewer.NewEventLogGCService(eventLogStore, gcPath, cfg.ViewerLog.RetentionDays, cfg.ViewerLog.GCIntervalMinutes); err != nil {
				log.Printf("WARN: viewer event log GC disabled: %v", err)
			} else {
				deps.eventLogGC = gcSvc
				deps.eventLogGC.Start()
				log.Printf("Viewer event log GC enabled: %s", gcPath)
			}
		}
	}
	if reportStore, err := executionpersistence.NewJSONLReportStore(reportPath); err != nil {
		deps.monitorStore = viewer.NewMonitorStore(nil, deps.eventLogStore)
		deps.eventRelay = &idleAwareEventListener{hub: hub, monitor: deps.monitorStore, archive: deps.eventLogStore}
		deps.viewerStatus = viewer.HandleMonitorStatus(deps.monitorStore)
		deps.viewerAgents = viewer.HandleMonitorAgents(deps.monitorStore)
		deps.viewerAgentDetail = viewer.HandleMonitorAgentDetail(deps.monitorStore)
		deps.viewerJobs = viewer.HandleMonitorJobs(deps.monitorStore)
		deps.viewerLogs = viewer.HandleMonitorLogs(deps.monitorStore)
		deps.viewerAuditSummary = viewer.HandleMonitorAuditSummary(deps.monitorStore)
		deps.viewerJobDetail = viewer.HandleMonitorJobDetail(deps.monitorStore)
		log.Printf("WARN: evidence API disabled: %v", err)
	} else {
		deps.reportStore = reportStore
		deps.monitorStore = viewer.NewMonitorStore(reportStore, deps.eventLogStore)
		deps.eventRelay = &idleAwareEventListener{hub: hub, monitor: deps.monitorStore, archive: deps.eventLogStore}
		deps.viewerStatus = viewer.HandleMonitorStatus(deps.monitorStore)
		deps.viewerAgents = viewer.HandleMonitorAgents(deps.monitorStore)
		deps.viewerAgentDetail = viewer.HandleMonitorAgentDetail(deps.monitorStore)
		deps.viewerJobs = viewer.HandleMonitorJobs(deps.monitorStore)
		deps.viewerLogs = viewer.HandleMonitorLogs(deps.monitorStore)
		deps.viewerAuditSummary = viewer.HandleMonitorAuditSummary(deps.monitorStore)
		deps.viewerJobDetail = viewer.HandleMonitorJobDetail(deps.monitorStore)
		deps.evidenceHandler = viewer.HandleEvidenceRecent(reportStore)
		deps.evidenceDetail = viewer.HandleEvidenceDetail(reportStore)
		deps.evidenceSummary = viewer.HandleEvidenceSummary(reportStore)
		log.Printf("Viewer evidence API enabled: %s", reportPath)
	}
}
