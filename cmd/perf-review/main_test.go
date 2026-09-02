package main

import (
	"reflect"
	"testing"
)

func TestParseOptions(t *testing.T) {
	opts, err := parseOptions([]string{
		"-urls", "https://one.test, https://two.test",
		"-iterations", "5",
		"-include-mutations=false",
		"-include-scroll=false",
		"-json",
		"-v",
	})
	if err != nil {
		t.Fatal(err)
	}

	wantURLs := []string{"https://one.test", "https://two.test"}
	if !reflect.DeepEqual(opts.urls, wantURLs) {
		t.Fatalf("urls = %v, want %v", opts.urls, wantURLs)
	}
	if opts.iterations != 5 || opts.includeMutations || opts.includeScroll || !opts.json || !opts.verbose {
		t.Fatalf("unexpected options: %+v", opts)
	}
	if opts.fixtures != "" {
		t.Fatalf("fixtures = %q, want empty", opts.fixtures)
	}
}

func TestParseOptionsFixtures(t *testing.T) {
	opts, err := parseOptions([]string{
		"-fixtures", "testdata/perf/",
		"-iterations", "1",
		"-include-mutations=false",
		"-include-scroll=false",
		"-json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.fixtures != "testdata/perf/" {
		t.Fatalf("fixtures = %q, want testdata/perf/", opts.fixtures)
	}
	if opts.iterations != 1 {
		t.Fatalf("iterations = %d, want 1", opts.iterations)
	}
	if !opts.json {
		t.Fatal("json should be true")
	}
}

func TestParseOptionsRejectsInvalidInput(t *testing.T) {
	tests := map[string][]string{
		"non-positive iterations": {"-iterations", "0"},
		"unknown flag":            {"-unknown"},
		"positional argument":     {"extra"},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := parseOptions(args); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
