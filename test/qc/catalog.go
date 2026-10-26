package qc

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

const expectedCaseCount = 257

var caseLine = regexp.MustCompile(`^- \*\*(BR-[0-9]{3}) \| (P[0-2]) \| ([^|]+) \| ([^*]+)\*\* — (.*)$`)

type Case struct {
	ID          string   `json:"id"`
	Priority    string   `json:"priority"`
	Methods     []string `json:"methods"`
	Status      string   `json:"status"`
	Description string   `json:"description"`
}

func parseCatalog(path string) ([]Case, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var cases []Case
	seen := make(map[string]bool)
	s := bufio.NewScanner(f)
	for lineNo := 1; s.Scan(); lineNo++ {
		line := s.Text()
		if !strings.HasPrefix(line, "- **BR-") {
			continue
		}
		m := caseLine.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("%s:%d: malformed QC case row", path, lineNo)
		}
		if seen[m[1]] {
			return nil, fmt.Errorf("%s:%d: duplicate QC case %s", path, lineNo, m[1])
		}
		seen[m[1]] = true
		methods := strings.Split(m[3], ",")
		for i := range methods {
			methods[i] = strings.TrimSpace(methods[i])
		}
		cases = append(cases, Case{
			ID:          m[1],
			Priority:    m[2],
			Methods:     methods,
			Status:      strings.TrimSpace(m[4]),
			Description: strings.TrimSpace(m[5]),
		})
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if len(cases) != expectedCaseCount {
		return nil, fmt.Errorf("%s: parsed %d QC cases, want %d", path, len(cases), expectedCaseCount)
	}
	return cases, nil
}
