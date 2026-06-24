package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	longjobapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/longjob"
	longjobdomain "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/longjob"
)

const jobDirEnv = "RENCROW_JOB_DIR"

type jobCLIDeps struct {
	service *longjobapp.Service
	out     io.Writer
	errOut  io.Writer
}

func cmdJob() {
	store := longjobapp.NewFileStore(defaultJobDir())
	deps := jobCLIDeps{
		service: longjobapp.NewService(store, func() time.Time { return time.Now().UTC() }),
		out:     os.Stdout,
		errOut:  os.Stderr,
	}
	os.Exit(runJobCommand(os.Args[2:], deps))
}

func runJobCommand(args []string, deps jobCLIDeps) int {
	if deps.out == nil {
		deps.out = os.Stdout
	}
	if deps.errOut == nil {
		deps.errOut = os.Stderr
	}
	if deps.service == nil {
		store := longjobapp.NewFileStore(defaultJobDir())
		deps.service = longjobapp.NewService(store, func() time.Time { return time.Now().UTC() })
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printJobHelp(deps.out)
		return 0
	}
	ctx := context.Background()
	switch args[0] {
	case "start":
		return runJobStart(ctx, args[1:], deps)
	case "list":
		return runJobList(ctx, args[1:], deps)
	case "status":
		return runJobStatus(ctx, args[1:], deps)
	case "resume":
		return runJobResume(ctx, args[1:], deps)
	case "complete-step":
		return runJobCompleteStep(ctx, args[1:], deps)
	case "report":
		return runJobReport(ctx, args[1:], deps)
	case "cancel":
		return runJobCancel(ctx, args[1:], deps)
	default:
		fmt.Fprintf(deps.errOut, "unknown job command: %s\n", args[0])
		printJobHelp(deps.errOut)
		return 2
	}
}

func runJobStart(ctx context.Context, args []string, deps jobCLIDeps) int {
	if len(args) == 0 {
		fmt.Fprintln(deps.errOut, "usage: rencrow job start stock-learn [--universe NAME] [--period RANGE] [--objective TEXT] [--goal TEXT] [--json]")
		return 2
	}
	kind := args[0]
	if kind != "stock-learn" {
		fmt.Fprintf(deps.errOut, "unsupported job kind: %s\n", kind)
		return 2
	}
	fs := flag.NewFlagSet("job start stock-learn", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var req longjobapp.StockLearnRequest
	jsonOut := false
	fs.StringVar(&req.Universe, "universe", "us-liquid", "stock universe")
	fs.StringVar(&req.Period, "period", "5y", "learning period")
	fs.StringVar(&req.Objective, "objective", "research", "research objective")
	fs.StringVar(&req.Goal, "goal", "", "job goal")
	fs.BoolVar(&jsonOut, "json", false, "write JSON")
	if err := fs.Parse(args[1:]); err != nil {
		fmt.Fprintln(deps.errOut, "usage: rencrow job start stock-learn [--universe NAME] [--period RANGE] [--objective TEXT] [--goal TEXT] [--json]")
		return 2
	}
	job, err := deps.service.StartStockLearn(ctx, req)
	if err != nil {
		fmt.Fprintf(deps.errOut, "job start failed: %v\n", err)
		return 1
	}
	if jsonOut {
		writeJSONCLI(deps.out, job, true)
		return 0
	}
	fmt.Fprintf(deps.out, "job started: %s\nkind: %s\nstatus: %s\nnext: %s\n", job.ID, job.Kind, job.Status, jobResumePointOrNext(job))
	return 0
}

func runJobList(ctx context.Context, args []string, deps jobCLIDeps) int {
	jsonOut := hasFlag(args, "--json")
	jobs, err := deps.service.List(ctx)
	if err != nil {
		fmt.Fprintf(deps.errOut, "job list failed: %v\n", err)
		return 1
	}
	if jsonOut {
		writeJSONCLI(deps.out, map[string]any{"jobs": jobs}, true)
		return 0
	}
	if len(jobs) == 0 {
		fmt.Fprintln(deps.out, "no jobs")
		return 0
	}
	for _, job := range jobs {
		fmt.Fprintf(deps.out, "%s\t%s\t%s\t%d/%d\t%s\n", job.ID, job.Kind, job.Status, job.CompletedStepCount(), len(job.Plan), job.Goal)
	}
	return 0
}

func runJobStatus(ctx context.Context, args []string, deps jobCLIDeps) int {
	id, jsonOut, ok := parseJobIDAndJSON(args)
	if !ok {
		fmt.Fprintln(deps.errOut, "usage: rencrow job status <job_id> [--json]")
		return 2
	}
	job, err := deps.service.Load(ctx, id)
	if err != nil {
		fmt.Fprintf(deps.errOut, "job status failed: %v\n", err)
		return 1
	}
	if jsonOut {
		writeJSONCLI(deps.out, job, true)
		return 0
	}
	printJobStatus(deps.out, job)
	return 0
}

func runJobResume(ctx context.Context, args []string, deps jobCLIDeps) int {
	id, jsonOut, ok := parseJobIDAndJSON(args)
	if !ok {
		fmt.Fprintln(deps.errOut, "usage: rencrow job resume <job_id> [--json]")
		return 2
	}
	res, err := deps.service.Resume(ctx, id)
	if err != nil {
		fmt.Fprintf(deps.errOut, "job resume failed: %v\n", err)
		return 1
	}
	if jsonOut {
		writeJSONCLI(deps.out, res, true)
		return 0
	}
	fmt.Fprintf(deps.out, "resume point: %s\nrole: %s\nartifact: %s\n\n%s", res.Step.ID, res.Step.Role, res.ArtifactPath, res.Prompt)
	return 0
}

func runJobCompleteStep(ctx context.Context, args []string, deps jobCLIDeps) int {
	id, stepID, summary, artifact, jsonOut, ok := parseCompleteStepArgs(args)
	if !ok {
		fmt.Fprintln(deps.errOut, "usage: rencrow job complete-step <job_id> [--step ID] [--summary TEXT] [--artifact PATH] [--json]")
		return 2
	}
	job, err := deps.service.CompleteStep(ctx, id, stepID, summary, artifact)
	if err != nil {
		fmt.Fprintf(deps.errOut, "job complete-step failed: %v\n", err)
		return 1
	}
	if jsonOut {
		writeJSONCLI(deps.out, job, true)
		return 0
	}
	fmt.Fprintf(deps.out, "step completed\njob: %s\nstatus: %s\nnext: %s\n", job.ID, job.Status, jobResumePointOrNext(job))
	return 0
}

func runJobReport(ctx context.Context, args []string, deps jobCLIDeps) int {
	id, jsonOut, ok := parseJobIDAndJSON(args)
	if !ok {
		fmt.Fprintln(deps.errOut, "usage: rencrow job report <job_id> [--json]")
		return 2
	}
	job, err := deps.service.Load(ctx, id)
	if err != nil {
		fmt.Fprintf(deps.errOut, "job report failed: %v\n", err)
		return 1
	}
	if jsonOut {
		writeJSONCLI(deps.out, job, true)
		return 0
	}
	fmt.Fprint(deps.out, formatJobReport(job))
	return 0
}

func runJobCancel(ctx context.Context, args []string, deps jobCLIDeps) int {
	id, reason, jsonOut, ok := parseCancelArgs(args)
	if !ok {
		fmt.Fprintln(deps.errOut, "usage: rencrow job cancel <job_id> [--reason TEXT] [--json]")
		return 2
	}
	job, err := deps.service.Cancel(ctx, id, reason)
	if err != nil {
		fmt.Fprintf(deps.errOut, "job cancel failed: %v\n", err)
		return 1
	}
	if jsonOut {
		writeJSONCLI(deps.out, job, true)
		return 0
	}
	fmt.Fprintf(deps.out, "job canceled: %s\n", job.ID)
	return 0
}

func parseCompleteStepArgs(args []string) (id, stepID, summary, artifact string, jsonOut, ok bool) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOut = true
		case "--step", "--summary", "--artifact":
			if i+1 >= len(args) {
				return "", "", "", "", false, false
			}
			value := args[i+1]
			i++
			switch args[i-1] {
			case "--step":
				stepID = value
			case "--summary":
				summary = value
			case "--artifact":
				artifact = value
			}
		default:
			if strings.HasPrefix(args[i], "-") || id != "" {
				return "", "", "", "", false, false
			}
			id = args[i]
		}
	}
	return id, stepID, summary, artifact, jsonOut, strings.TrimSpace(id) != ""
}

func parseCancelArgs(args []string) (id, reason string, jsonOut, ok bool) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOut = true
		case "--reason":
			if i+1 >= len(args) {
				return "", "", false, false
			}
			reason = args[i+1]
			i++
		default:
			if strings.HasPrefix(args[i], "-") || id != "" {
				return "", "", false, false
			}
			id = args[i]
		}
	}
	return id, reason, jsonOut, strings.TrimSpace(id) != ""
}

func parseJobIDAndJSON(args []string) (string, bool, bool) {
	jsonOut := false
	var positional []string
	for _, arg := range args {
		if arg == "--json" {
			jsonOut = true
			continue
		}
		positional = append(positional, arg)
	}
	if len(positional) != 1 || strings.TrimSpace(positional[0]) == "" {
		return "", false, false
	}
	return positional[0], jsonOut, true
}

func printJobHelp(out io.Writer) {
	fmt.Fprint(out, `Usage: rencrow job <command>

Commands:
  start stock-learn  Create a resumable stock-learning research job
  list               List jobs
  status <job_id>    Show job state and next step
  resume <job_id>    Mark/return the next step and write a resume prompt artifact
  complete-step      Mark the current or selected step completed
  report <job_id>    Print a Markdown job report
  cancel <job_id>    Cancel a job

Environment:
  RENCROW_JOB_DIR    Override job storage directory (default: ~/.rencrow/jobs)
`)
}

func printJobStatus(out io.Writer, job longjobdomain.Job) {
	fmt.Fprintf(out, "job: %s\nkind: %s\nstatus: %s\nprogress: %d/%d\nnext: %s\ngoal: %s\n",
		job.ID, job.Kind, job.Status, job.CompletedStepCount(), len(job.Plan), jobResumePointOrNext(job), job.Goal)
	for _, step := range job.Plan {
		fmt.Fprintf(out, "- [%s] %s (%s): %s\n", step.Status, step.ID, step.Role, step.Title)
	}
}

func formatJobReport(job longjobdomain.Job) string {
	var b strings.Builder
	b.WriteString("# RenCrow Long Running Job Report\n\n")
	b.WriteString(fmt.Sprintf("- job_id: %s\n- kind: %s\n- status: %s\n- progress: %d/%d\n- goal: %s\n\n",
		job.ID, job.Kind, job.Status, job.CompletedStepCount(), len(job.Plan), job.Goal))
	b.WriteString("## Plan\n")
	for _, step := range job.Plan {
		b.WriteString(fmt.Sprintf("- [%s] %s (%s): %s", step.Status, step.ID, step.Role, step.Title))
		if step.Summary != "" {
			b.WriteString(" - " + step.Summary)
		}
		b.WriteString("\n")
	}
	if len(job.Artifacts) > 0 {
		b.WriteString("\n## Artifacts\n")
		for _, artifact := range job.Artifacts {
			b.WriteString(fmt.Sprintf("- %s (%s): %s\n", artifact.ID, artifact.Kind, artifact.Path))
		}
	}
	if len(job.SharedContext) > 0 {
		b.WriteString("\n## Shared Context\n")
		for _, entry := range job.SharedContext {
			b.WriteString(fmt.Sprintf("- %s: %s\n", entry.Role, entry.Content))
		}
	}
	return b.String()
}

func defaultJobDir() string {
	if dir := strings.TrimSpace(os.Getenv(jobDirEnv)); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".", ".rencrow", "jobs")
	}
	return filepath.Join(home, ".rencrow", "jobs")
}

func jobResumePointOrNext(job longjobdomain.Job) string {
	if strings.TrimSpace(job.ResumePoint) != "" {
		return job.ResumePoint
	}
	idx := job.NextStepIndex()
	if idx < 0 {
		return ""
	}
	return job.Plan[idx].ID
}
