package wpt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const DefaultWPTCommit = "11ae8c1179be2dd38070846b01c2e9338ef81770"

const WPTRepo = "https://github.com/web-platform-tests/wpt"

type TestType string

const (
	TestHarness TestType = "testharness"

	RefTest TestType = "reftest"

	ManualTest TestType = "manual"

	SmokeTest TestType = "smoke"

	SupportFile TestType = "support"

	UnclassifiedTest TestType = "unclassified"

	VisualTest TestType = "visual"
)

type TestResult struct {
	Path        string              `json:"path"`
	Type        TestType            `json:"type"`
	Status      string              `json:"status"`
	Score       float64             `json:"score,omitempty"`
	Message     string              `json:"message,omitempty"`
	DurationMS  int64               `json:"duration_ms,omitempty"`
	RefPath     string              `json:"ref_path,omitempty"`
	RefRelation string              `json:"ref_relation,omitempty"`
	Variants    []TestVariantResult `json:"variants,omitempty"`
}

type TestVariantResult struct {
	Query      string `json:"query"`
	Status     string `json:"status"`
	Message    string `json:"message,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
}

type SuiteResult struct {
	Commit    string       `json:"commit"`
	Timestamp string       `json:"timestamp"`
	Total     int          `json:"total"`
	Passed    int          `json:"passed"`
	Failed    int          `json:"failed"`
	Skipped   int          `json:"skipped"`
	Errored   int          `json:"errored"`
	PassRate  float64      `json:"pass_rate"`
	Results   []TestResult `json:"results"`
	Duration  float64      `json:"duration_s"`
}

func (r SuiteResult) MeetsMinPassRate(min float64) bool {
	if min <= 0 {
		return true
	}
	if r.Total-r.Skipped <= 0 {
		return false
	}
	return r.PassRate >= min
}

func CheckSuiteThreshold(result SuiteResult, minRate float64) error {
	if minRate <= 0 {
		return nil
	}
	if result.MeetsMinPassRate(minRate) {
		return nil
	}
	return fmt.Errorf("wpt: pass rate %.1f%% below floor %.1f%% (%d/%d passed, %d failed, %d errored)",
		result.PassRate, minRate, result.Passed, result.Total-result.Skipped,
		result.Failed, result.Errored)
}

type Config struct {
	WPTDir             string              `json:"wpt_dir"`
	Commit             string              `json:"commit"`
	Directories        []string            `json:"directories"`
	Include            []string            `json:"include"`
	TestVariants       map[string][]string `json:"test_variants"`
	Exclude            []string            `json:"exclude"`
	PassThreshold      float64             `json:"pass_threshold"`
	MinPassRate        float64             `json:"min_pass_rate"`
	ViewportWidth      int                 `json:"viewport_width"`
	ViewportHeight     int                 `json:"viewport_height"`
	DPR                float64             `json:"dpr"`
	Timeout            int                 `json:"timeout"`
	GoosieBinary       string              `json:"goosie_binary"`
	OutputDir          string              `json:"output_dir"`
	ReferenceDir       string              `json:"reference_dir"`
	GenerateReferences bool                `json:"generate_references"`
}

func wptSuiteEnabled(getenv func(string) string) bool {
	switch strings.ToLower(strings.TrimSpace(getenv("WPT_RUN_SUITE"))) {
	case "1", "true", "yes":
		return true
	}
	return false
}

func DefaultConfig() Config {
	return Config{
		Commit:         DefaultWPTCommit,
		PassThreshold:  95.0,
		ViewportWidth:  800,
		ViewportHeight: 600,
		DPR:            1.0,
		Timeout:        30,
		GoosieBinary:   "./goosie",
		OutputDir:      "/tmp/wpt-results",
		ReferenceDir:   "/tmp/wpt-references",
	}
}

func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("wpt: read config %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("wpt: parse config %s: %w", path, err)
	}
	if cfg.Commit == "" {
		cfg.Commit = DefaultWPTCommit
	}
	if cfg.PassThreshold <= 0 {
		cfg.PassThreshold = 95.0
	}
	if cfg.ViewportWidth <= 0 {
		cfg.ViewportWidth = 800
	}
	if cfg.ViewportHeight <= 0 {
		cfg.ViewportHeight = 600
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30
	}
	return cfg, nil
}

func (c *Config) WPTCheckoutDir() string {
	if c.WPTDir != "" {
		return c.WPTDir
	}
	return filepath.Join(os.TempDir(), "goosie-wpt")
}
