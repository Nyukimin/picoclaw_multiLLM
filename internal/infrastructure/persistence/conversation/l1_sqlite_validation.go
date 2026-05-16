package conversation

import (
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

func validateMemoryState(memoryState string) error {
	switch memoryState {
	case MemoryStateObserved, MemoryStateCandidate, MemoryStateConfirmed:
		return nil
	default:
		return fmt.Errorf("invalid l1 memory state: %s", memoryState)
	}
}

func validateL1StagingKind(kind string) error {
	switch kind {
	case L1StagingKindExternalFetch, L1StagingKindMemoryCandidate, L1StagingKindSearchResult:
		return nil
	default:
		return fmt.Errorf("invalid l1 staging kind: %s", kind)
	}
}

func validateL1StagingStatus(status string) error {
	switch status {
	case L1StagingStatusPending, L1StagingStatusValidated, L1StagingStatusRejected:
		return nil
	default:
		return fmt.Errorf("invalid l1 staging validation status: %s", status)
	}
}

func validateL1SourceKind(kind string) error {
	switch kind {
	case L1SourceKindRSS, L1SourceKindAtom, L1SourceKindOfficialAPI, L1SourceKindGitHub,
		L1SourceKindHuggingFace, L1SourceKindPyPI, L1SourceKindMediaWiki, L1SourceKindSearchFallback:
		return nil
	default:
		return fmt.Errorf("invalid l1 source registry kind: %s", kind)
	}
}
