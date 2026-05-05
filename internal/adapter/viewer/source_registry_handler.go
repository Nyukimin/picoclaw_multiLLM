package viewer

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	conversationpersistence "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/conversation"
	"gopkg.in/yaml.v3"
)

type SourceRegistryStore interface {
	SaveSourceRegistryEntry(ctx context.Context, entry conversationpersistence.L1SourceRegistryEntry) (*conversationpersistence.L1SourceRegistryEntry, error)
	ListSourceRegistryEntries(ctx context.Context, enabledOnly bool) ([]conversationpersistence.L1SourceRegistryEntry, error)
}

type sourceRegistryEntryDTO struct {
	SourceID         string         `json:"source_id" yaml:"source_id"`
	URL              string         `json:"url" yaml:"url"`
	Kind             string         `json:"kind" yaml:"kind"`
	TrustScore       float64        `json:"trust_score" yaml:"trust_score"`
	FetchIntervalSec int64          `json:"fetch_interval_sec" yaml:"fetch_interval_sec"`
	LicenseNote      string         `json:"license_note" yaml:"license_note"`
	Enabled          bool           `json:"enabled" yaml:"enabled"`
	Meta             map[string]any `json:"meta,omitempty" yaml:"meta,omitempty"`
	LastFetchedAt    string         `json:"last_fetched_at,omitempty" yaml:"last_fetched_at,omitempty"`
	LastStatus       string         `json:"last_status,omitempty" yaml:"last_status,omitempty"`
	LastError        string         `json:"last_error,omitempty" yaml:"last_error,omitempty"`
}

type sourceRegistryPayload struct {
	Entries []sourceRegistryEntryDTO `json:"entries" yaml:"entries"`
}

func HandleSourceRegistry(store SourceRegistryStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			http.Error(w, "source registry unavailable", http.StatusServiceUnavailable)
			return
		}
		switch r.Method {
		case http.MethodGet:
			handleSourceRegistryList(w, r, store)
		case http.MethodPost:
			handleSourceRegistrySave(w, r, store)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func handleSourceRegistryList(w http.ResponseWriter, r *http.Request, store SourceRegistryStore) {
	enabledOnly := r.URL.Query().Get("enabled_only") == "1" || strings.EqualFold(r.URL.Query().Get("enabled_only"), "true")
	entries, err := store.ListSourceRegistryEntries(r.Context(), enabledOnly)
	if err != nil {
		http.Error(w, "failed to list source registry", http.StatusInternalServerError)
		return
	}
	payload := sourceRegistryPayload{Entries: sourceRegistryEntriesToDTO(entries)}
	if wantsYAML(r) {
		w.Header().Set("Content-Type", "application/x-yaml")
		_ = yaml.NewEncoder(w).Encode(payload)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func handleSourceRegistrySave(w http.ResponseWriter, r *http.Request, store SourceRegistryStore) {
	payload, err := decodeSourceRegistryPayload(r)
	if err != nil {
		http.Error(w, "invalid source registry payload", http.StatusBadRequest)
		return
	}
	saved := make([]sourceRegistryEntryDTO, 0, len(payload.Entries))
	for _, dto := range payload.Entries {
		entry, err := store.SaveSourceRegistryEntry(r.Context(), dto.toEntry())
		if err != nil {
			http.Error(w, "failed to save source registry", http.StatusBadRequest)
			return
		}
		saved = append(saved, sourceRegistryEntryToDTO(*entry))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sourceRegistryPayload{Entries: saved})
}

func decodeSourceRegistryPayload(r *http.Request) (sourceRegistryPayload, error) {
	var payload sourceRegistryPayload
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return payload, err
	}
	if wantsYAML(r) {
		if err := yaml.Unmarshal(body, &payload); err != nil {
			return payload, err
		}
		return payload, nil
	}
	if err := json.Unmarshal(body, &payload); err == nil && len(payload.Entries) > 0 {
		return payload, nil
	}
	var single sourceRegistryEntryDTO
	if err := json.Unmarshal(body, &single); err != nil {
		return payload, err
	}
	payload.Entries = []sourceRegistryEntryDTO{single}
	return payload, nil
}

func wantsYAML(r *http.Request) bool {
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "yaml" || format == "yml" {
		return true
	}
	ct := strings.ToLower(r.Header.Get("Content-Type"))
	return strings.Contains(ct, "yaml") || strings.Contains(ct, "yml")
}

func sourceRegistryEntriesToDTO(entries []conversationpersistence.L1SourceRegistryEntry) []sourceRegistryEntryDTO {
	out := make([]sourceRegistryEntryDTO, 0, len(entries))
	for _, entry := range entries {
		out = append(out, sourceRegistryEntryToDTO(entry))
	}
	return out
}

func sourceRegistryEntryToDTO(entry conversationpersistence.L1SourceRegistryEntry) sourceRegistryEntryDTO {
	dto := sourceRegistryEntryDTO{
		SourceID:         entry.SourceID,
		URL:              entry.URL,
		Kind:             entry.Kind,
		TrustScore:       entry.TrustScore,
		FetchIntervalSec: int64(entry.FetchInterval.Seconds()),
		LicenseNote:      entry.LicenseNote,
		Enabled:          entry.Enabled,
		Meta:             entry.Meta,
		LastStatus:       entry.LastStatus,
		LastError:        entry.LastError,
	}
	if !entry.LastFetchedAt.IsZero() {
		dto.LastFetchedAt = entry.LastFetchedAt.UTC().Format(time.RFC3339)
	}
	return dto
}

func (dto sourceRegistryEntryDTO) toEntry() conversationpersistence.L1SourceRegistryEntry {
	return conversationpersistence.L1SourceRegistryEntry{
		SourceID:      dto.SourceID,
		URL:           dto.URL,
		Kind:          dto.Kind,
		TrustScore:    dto.TrustScore,
		FetchInterval: time.Duration(dto.FetchIntervalSec) * time.Second,
		LicenseNote:   dto.LicenseNote,
		Enabled:       dto.Enabled,
		Meta:          dto.Meta,
	}
}
