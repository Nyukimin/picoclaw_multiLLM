package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/pkg/modules"
)

func TestAggregationModule(t *testing.T) {
	module := NewAggregationModule()

	// Test initialization
	agent := &modules.AgentCore{
		ID:    "worker",
		Alias: "Shiro",
	}

	err := module.Initialize(context.Background(), agent)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Test Name
	if module.Name() != "Aggregation" {
		t.Errorf("Expected name 'Aggregation', got '%s'", module.Name())
	}

	// Test single result aggregation
	results := []OrderResult{
		{
			JobID:   "job_20260301_001",
			OrderID: "order1",
			Success: true,
			Output:  "Test output",
		},
	}

	aggregated, err := module.Aggregate(context.Background(), results)
	if err != nil {
		t.Fatalf("Aggregate failed: %v", err)
	}

	if aggregated.OrderID != "order1" {
		t.Errorf("Expected OrderID 'order1', got '%s'", aggregated.OrderID)
	}

	// Test multiple results - first successful
	results = []OrderResult{
		{
			JobID:   "job_20260301_002",
			OrderID: "order1",
			Success: true,
			Output:  "First output",
		},
		{
			JobID:   "job_20260301_002",
			OrderID: "order2",
			Success: true,
			Output:  "Second output",
		},
	}

	aggregated, err = module.Aggregate(context.Background(), results)
	if err != nil {
		t.Fatalf("Aggregate failed: %v", err)
	}

	if aggregated.OrderID != "order1" {
		t.Errorf("Expected first successful result (order1), got '%s'", aggregated.OrderID)
	}

	// Test multiple results - all failed
	results = []OrderResult{
		{
			JobID:   "job_20260301_003",
			OrderID: "order1",
			Success: false,
			Error:   errors.New("error1"),
		},
		{
			JobID:   "job_20260301_003",
			OrderID: "order2",
			Success: false,
			Error:   errors.New("error2"),
		},
	}

	aggregated, err = module.Aggregate(context.Background(), results)
	if err != nil {
		t.Fatalf("Aggregate failed: %v", err)
	}

	if aggregated.OrderID != "order1" {
		t.Errorf("Expected first result when all failed (order1), got '%s'", aggregated.OrderID)
	}

	// Test empty results
	results = []OrderResult{}
	_, err = module.Aggregate(context.Background(), results)
	if err == nil {
		t.Error("Expected error for empty results")
	}

	// Test Shutdown
	err = module.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}
