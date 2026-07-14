package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	binaryPath = "/tmp/goosie-browser"
	buildPath  = "./cmd/browser"
	docPath    = "docs/BINARY_SIZES.md"
)

func main() {
	// 1. Build the binary
	fmt.Println("Building binary...")
	cmd := exec.Command("go", "build", "-o", binaryPath, buildPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed to build binary: %v", err)
	}

	// 2. Record binary size
	info, err := os.Stat(binaryPath)
	if err != nil {
		log.Fatalf("Failed to stat binary: %v", err)
	}
	sizeBytes := info.Size()
	fmt.Printf("Binary size: %d bytes\n", sizeBytes)

	// 3. Gather build info
	dateStr := time.Now().Format("2006-01-02")
	commitHash := getCommitHash()
	goVersion := getGoVersion()
	osArch := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)

	// 4. Read previous entry, check growth, and append
	processDocs(sizeBytes, dateStr, commitHash, goVersion, osArch)
}

func getCommitHash() string {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func getGoVersion() string {
	cmd := exec.Command("go", "env", "GOVERSION")
	out, err := cmd.Output()
	if err != nil {
		return runtime.Version()
	}
	return strings.TrimSpace(string(out))
}

func processDocs(currentSize int64, dateStr, commitHash, goVersion, osArch string) {
	// Ensure docs directory exists
	if err := os.MkdirAll(filepath.Dir(docPath), 0755); err != nil {
		log.Fatalf("Failed to create docs directory: %v", err)
	}

	var previousSize int64 = 0
	var fileExists bool

	// Check if file exists and read last entry
	if _, err := os.Stat(docPath); err == nil {
		fileExists = true
		prev := getPreviousSize(docPath)
		if prev > 0 {
			previousSize = prev
		}
	}

	// Calculate percentage change
	changeStr := "0.00%"
	if previousSize > 0 {
		diff := float64(currentSize) - float64(previousSize)
		pct := (diff / float64(previousSize)) * 100.0
		if pct > 0 {
			changeStr = fmt.Sprintf("+%.2f%%", pct)
		} else {
			changeStr = fmt.Sprintf("%.2f%%", pct)
		}

		if pct > 5.0 {
			fmt.Fprintf(os.Stderr, "WARNING: Binary size grew by %.2f%%! (Previous: %d, Current: %d)\n", pct, previousSize, currentSize)
		}
	} else {
		changeStr = "N/A"
	}

	// Format new row
	// | YYYY-MM-DD | commit hash | Go version | OS/arch | N bytes | ±X% |
	newRow := fmt.Sprintf("| %s | %s | %s | %s | %d | %s |\n", dateStr, commitHash, goVersion, osArch, currentSize, changeStr)

	// Append or create
	f, err := os.OpenFile(docPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open %s: %v", docPath, err)
	}
	defer f.Close()

	if !fileExists {
		header := "# Binary Size Tracking\n\n| Date | Commit | Go Version | OS/Arch | Size (bytes) | Change |\n|---|---|---|---|---|---|\n"
		if _, err := f.WriteString(header); err != nil {
			log.Fatalf("Failed to write header: %v", err)
		}
	}

	if _, err := f.WriteString(newRow); err != nil {
		log.Fatalf("Failed to write new row: %v", err)
	}
	fmt.Printf("Appended entry to %s\n", docPath)
}

func getPreviousSize(path string) int64 {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0
	}

	lines := strings.Split(string(content), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "|") && !strings.Contains(line, "---|") && !strings.Contains(line, "Size (bytes)") {
			parts := strings.Split(line, "|")
			if len(parts) >= 7 {
				sizeStr := strings.TrimSpace(parts[5])
				size, err := strconv.ParseInt(sizeStr, 10, 64)
				if err == nil {
					return size
				}
			}
		}
	}
	return 0
}
