package longjob

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	domain "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/longjob"
)

type FileStore struct {
	root string
}

func NewFileStore(root string) *FileStore {
	return &FileStore{root: filepath.Clean(root)}
}

func (s *FileStore) Root() string {
	return s.root
}

func (s *FileStore) Save(_ context.Context, job domain.Job) error {
	if err := job.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(s.jobsDir(), 0o755); err != nil {
		return err
	}
	path := s.jobPath(job.ID)
	tmp := path + ".tmp"
	data, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *FileStore) Load(_ context.Context, id string) (domain.Job, error) {
	id = cleanJobID(id)
	if id == "" {
		return domain.Job{}, fmt.Errorf("job id is required")
	}
	data, err := os.ReadFile(s.jobPath(id))
	if err != nil {
		return domain.Job{}, err
	}
	var job domain.Job
	if err := json.Unmarshal(data, &job); err != nil {
		return domain.Job{}, err
	}
	return job, nil
}

func (s *FileStore) List(_ context.Context) ([]domain.Job, error) {
	entries, err := os.ReadDir(s.jobsDir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	jobs := make([]domain.Job, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.jobsDir(), entry.Name()))
		if err != nil {
			return nil, err
		}
		var job domain.Job
		if err := json.Unmarshal(data, &job); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].UpdatedAt.After(jobs[j].UpdatedAt)
	})
	return jobs, nil
}

func (s *FileStore) WriteArtifact(jobID, name string, data []byte) (string, error) {
	jobID = cleanJobID(jobID)
	if jobID == "" {
		return "", fmt.Errorf("job id is required")
	}
	name = cleanArtifactName(name)
	if name == "" {
		return "", fmt.Errorf("artifact name is required")
	}
	dir := filepath.Join(s.root, "artifacts", jobID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func (s *FileStore) jobsDir() string {
	return filepath.Join(s.root, "jobs")
}

func (s *FileStore) jobPath(id string) string {
	return filepath.Join(s.jobsDir(), cleanJobID(id)+".json")
}

func cleanJobID(id string) string {
	id = strings.TrimSpace(id)
	id = filepath.Base(id)
	id = strings.TrimSuffix(id, ".json")
	return id
}

func cleanArtifactName(name string) string {
	name = strings.TrimSpace(name)
	name = filepath.Base(name)
	name = strings.ReplaceAll(name, " ", "_")
	return name
}
