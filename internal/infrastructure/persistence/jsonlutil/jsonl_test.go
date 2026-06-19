package jsonlutil

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type testRecord struct {
	ID int `json:"id"`
}

func TestListLatestReadsTailNewestFirst(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.jsonl")
	for i := 1; i <= 5; i++ {
		if err := Append(path, testRecord{ID: i}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	items, err := ListLatest[testRecord](path, 3)
	if err != nil {
		t.Fatalf("ListLatest() error = %v", err)
	}
	if got := []int{items[0].ID, items[1].ID, items[2].ID}; got[0] != 5 || got[1] != 4 || got[2] != 3 {
		t.Fatalf("latest IDs = %v, want [5 4 3]", got)
	}
}

func TestAppendBoundedCompactsWhenFileExceedsMaxBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.jsonl")
	for i := 1; i <= 6; i++ {
		if err := AppendBounded(path, testRecord{ID: i}, BoundOptions{MaxRecords: 3, MaxBytes: 1}); err != nil {
			t.Fatalf("append bounded %d: %v", i, err)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read compacted file: %v", err)
	}
	lines := splitNonEmptyLines(data)
	if len(lines) != 3 {
		t.Fatalf("line count = %d, want 3; data=%s", len(lines), data)
	}
	var ids []int
	for _, line := range lines {
		var rec testRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatalf("unmarshal compacted line: %v", err)
		}
		ids = append(ids, rec.ID)
	}
	if ids[0] != 4 || ids[1] != 5 || ids[2] != 6 {
		t.Fatalf("compacted IDs = %v, want [4 5 6]", ids)
	}
}

func splitNonEmptyLines(data []byte) [][]byte {
	var out [][]byte
	start := 0
	for i, b := range data {
		if b != '\n' {
			continue
		}
		if i > start {
			out = append(out, data[start:i])
		}
		start = i + 1
	}
	if start < len(data) {
		out = append(out, data[start:])
	}
	return out
}
