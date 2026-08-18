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
