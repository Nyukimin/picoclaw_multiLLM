package worker

import (
	"context"
	"fmt"

	"github.com/Nyukimin/picoclaw_multiLLM/pkg/logger"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/modules"
)

// OrderResult represents the result from an Order agent (Order1/Order2/Order3).
type OrderResult struct {
	JobID      string
	OrderID    string
	Success    bool
	Output     string
	Plan       string
	Patch      string
	Risk       string
	Error      error
	Metadata   map[string]interface{}
}

// AggregationModule aggregates results from Order agents.
// When multiple Orders provide proposals (Deliberation Mode),
// this module collects and combines their results.
type AggregationModule struct {
	agent *modules.AgentCore
}

// NewAggregationModule creates a new aggregation module.
func NewAggregationModule() *AggregationModule {
	return &AggregationModule{}
}

// Name returns the module name.
func (m *AggregationModule) Name() string {
	return "Aggregation"
}

// Initialize initializes the module with the agent core.
func (m *AggregationModule) Initialize(ctx context.Context, agent *modules.AgentCore) error {
	m.agent = agent
	return nil
}

// Shutdown cleans up module resources.
func (m *AggregationModule) Shutdown(ctx context.Context) error {
	return nil
}

// Aggregate combines results from Order agents into a single result.
// For single Order execution, it just packages the result.
// For Deliberation Mode, it compares and ranks proposals.
func (m *AggregationModule) Aggregate(ctx context.Context, results []OrderResult) (OrderResult, error) {
	if len(results) == 0 {
		return OrderResult{}, fmt.Errorf("no results to aggregate")
	}

	logger.InfoCF("aggregation", "worker.aggregate", map[string]interface{}{
		"job_id":       results[0].JobID,
		"result_count": len(results),
	})

	// For single result, return as-is
	if len(results) == 1 {
		return results[0], nil
	}

	// For multiple results (Deliberation Mode), select the best one
	// TODO: Implement sophisticated comparison logic
	// For now, just return the first successful result
	for _, result := range results {
		if result.Success {
			return result, nil
		}
	}

	// If all failed, return the first error
	return results[0], nil
}
