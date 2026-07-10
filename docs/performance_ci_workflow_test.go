package docs_test

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPerformanceBenchmarkWorkflowRunsPRMicrobenchmarks(t *testing.T) {
	data, err := os.ReadFile("../.github/workflows/performance.yml")
	if err != nil {
		t.Fatalf("read workflow: %v", err)
	}

	var workflow workflowFile
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatalf("parse workflow yaml: %v", err)
	}

	if _, ok := workflow.On["pull_request"]; !ok {
		t.Fatalf("workflow must run on pull_request")
	}
	if _, ok := workflow.On["push"]; !ok {
		t.Fatalf("workflow must run on push to keep the main baseline exercised")
	}

	job, ok := workflow.Jobs["microbenchmarks"]
	if !ok {
		t.Fatalf("missing microbenchmarks job")
	}
	if job.RunsOn != "ubuntu-latest" {
		t.Fatalf("job runs-on = %q, want ubuntu-latest", job.RunsOn)
	}

	run := joinedRunCommands(job.Steps)
	for _, pkg := range []string{
		"./internal/dom",
		"./internal/css",
		"./internal/renderer",
	} {
		if !strings.Contains(run, pkg) {
			t.Fatalf("benchmark workflow does not run package %s:\n%s", pkg, run)
		}
	}

	for _, want := range []string{
		"-run=",
		"^$",
		"-benchmem",
		"-bench=",
		"BenchmarkParseSelector",
		"BenchmarkLayout",
		"BenchmarkDisplayList",
	} {
		if !strings.Contains(run, want) {
			t.Fatalf("benchmark workflow missing %q in run commands:\n%s", want, run)
		}
	}
}

type workflowFile struct {
	On   map[string]any         `yaml:"on"`
	Jobs map[string]workflowJob `yaml:"jobs"`
}

type workflowJob struct {
	RunsOn string         `yaml:"runs-on"`
	Steps  []workflowStep `yaml:"steps"`
}

type workflowStep struct {
	Run string `yaml:"run"`
}

func joinedRunCommands(steps []workflowStep) string {
	var builder strings.Builder
	for _, step := range steps {
		if step.Run == "" {
			continue
		}
		builder.WriteString(step.Run)
		builder.WriteByte('\n')
	}
	return builder.String()
}
