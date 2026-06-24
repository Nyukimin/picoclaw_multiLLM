package persona

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	domainpersona "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/persona"
)

type JSONLStore struct {
	discomfortPath  string
	triggerPath     string
	canonicalPath   string
	observationPath string
	metaUpdatePath  string
	sessionPath     string
	metaRoot        string
}

func NewJSONLStore(root string) *JSONLStore {
	if root == "" {
		root = "workspace/logs/persona"
	}
	return &JSONLStore{
		discomfortPath:  filepath.Join(root, "persona_discomfort_log.jsonl"),
		triggerPath:     filepath.Join(root, "persona_trigger_log.jsonl"),
		canonicalPath:   filepath.Join(root, "canonical_response_log.jsonl"),
		observationPath: filepath.Join(root, "observation_log.jsonl"),
		metaUpdatePath:  filepath.Join(root, "meta_profile_update.jsonl"),
		sessionPath:     filepath.Join(root, "persona_interface_session.jsonl"),
	}
}

func NewJSONLStoreWithMetaRoot(root, metaRoot string) *JSONLStore {
	store := NewJSONLStore(root)
	store.metaRoot = metaRoot
	return store
}

func (s *JSONLStore) SaveDiscomfortLog(_ context.Context, item domainpersona.DiscomfortLog) error {
	if err := domainpersona.ValidateDiscomfortLog(item); err != nil {
		return err
	}
	return appendJSONL(s.discomfortPath, item)
}

func (s *JSONLStore) ListDiscomfortLogs(_ context.Context, limit int) ([]domainpersona.DiscomfortLog, error) {
	if limit <= 0 {
		limit = 50
	}
	var items []domainpersona.DiscomfortLog
	if err := readJSONL(s.discomfortPath, func(line []byte) error {
		var item domainpersona.DiscomfortLog
		if err := json.Unmarshal(line, &item); err != nil {
			return err
		}
		items = append(items, item)
		return nil
	}); err != nil {
		return nil, err
	}
	return reverseLimit(items, limit), nil
}

func (s *JSONLStore) SaveTriggerLog(_ context.Context, item domainpersona.TriggerLog) error {
	if err := domainpersona.ValidateTriggerLog(item); err != nil {
		return err
	}
	return appendJSONL(s.triggerPath, item)
}

func (s *JSONLStore) ListTriggerLogs(_ context.Context, limit int) ([]domainpersona.TriggerLog, error) {
	if limit <= 0 {
		limit = 50
	}
	var items []domainpersona.TriggerLog
	if err := readJSONL(s.triggerPath, func(line []byte) error {
		var item domainpersona.TriggerLog
		if err := json.Unmarshal(line, &item); err != nil {
			return err
		}
		items = append(items, item)
		return nil
	}); err != nil {
		return nil, err
	}
	return reverseLimit(items, limit), nil
}

func (s *JSONLStore) SaveCanonicalResponseLog(_ context.Context, item domainpersona.CanonicalResponseLog) error {
	if err := domainpersona.ValidateCanonicalResponseLog(item); err != nil {
		return err
	}
	return appendJSONL(s.canonicalPath, item)
}

func (s *JSONLStore) ListCanonicalResponseLogs(_ context.Context, limit int) ([]domainpersona.CanonicalResponseLog, error) {
	if limit <= 0 {
		limit = 50
	}
	var items []domainpersona.CanonicalResponseLog
	if err := readJSONL(s.canonicalPath, func(line []byte) error {
		var item domainpersona.CanonicalResponseLog
		if err := json.Unmarshal(line, &item); err != nil {
			return err
		}
		items = append(items, item)
		return nil
	}); err != nil {
		return nil, err
	}
	return reverseLimit(items, limit), nil
}

func (s *JSONLStore) SaveObservationLog(_ context.Context, item domainpersona.ObservationLog) error {
	if err := domainpersona.ValidateObservationLog(item); err != nil {
		return err
	}
	return appendJSONL(s.observationPath, item)
}

func (s *JSONLStore) ListObservationLogs(_ context.Context, limit int) ([]domainpersona.ObservationLog, error) {
	if limit <= 0 {
		limit = 50
	}
	var items []domainpersona.ObservationLog
	if err := readJSONL(s.observationPath, func(line []byte) error {
		var item domainpersona.ObservationLog
		if err := json.Unmarshal(line, &item); err != nil {
			return err
		}
		items = append(items, item)
		return nil
	}); err != nil {
		return nil, err
	}
	return reverseLimit(items, limit), nil
}

func (s *JSONLStore) SaveMetaProfileUpdate(_ context.Context, item domainpersona.MetaProfileUpdate) error {
	if err := domainpersona.ValidateMetaProfileUpdate(item); err != nil {
		return err
	}
	return appendJSONL(s.metaUpdatePath, item)
}

func (s *JSONLStore) ListMetaProfileUpdates(_ context.Context, limit int) ([]domainpersona.MetaProfileUpdate, error) {
	if limit <= 0 {
		limit = 50
	}
	var items []domainpersona.MetaProfileUpdate
	if err := readJSONL(s.metaUpdatePath, func(line []byte) error {
		var item domainpersona.MetaProfileUpdate
		if err := json.Unmarshal(line, &item); err != nil {
			return err
		}
		items = append(items, item)
		return nil
	}); err != nil {
		return nil, err
	}
	return reverseLimit(items, limit), nil
}

func (s *JSONLStore) SaveInterfaceSession(_ context.Context, item domainpersona.InterfaceSession) error {
	if err := domainpersona.ValidateInterfaceSession(item); err != nil {
		return err
	}
	return appendJSONL(s.sessionPath, item)
}

func (s *JSONLStore) ListInterfaceSessions(_ context.Context, limit int) ([]domainpersona.InterfaceSession, error) {
	if limit <= 0 {
		limit = 50
	}
	var items []domainpersona.InterfaceSession
	if err := readJSONL(s.sessionPath, func(line []byte) error {
		var item domainpersona.InterfaceSession
		if err := json.Unmarshal(line, &item); err != nil {
			return err
		}
		items = append(items, item)
		return nil
	}); err != nil {
		return nil, err
	}
	return reverseLimit(items, limit), nil
}

func appendJSONL(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	line, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return err
}

func readJSONL(path string, fn func([]byte) error) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if err := fn(scanner.Bytes()); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func reverseLimit[T any](items []T, limit int) []T {
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	out := make([]T, 0, limit)
	for i := len(items) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, items[i])
	}
	return out
}
