package net

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type DownloadStatus string

const (
	DownloadRunning  DownloadStatus = "running"
	DownloadComplete DownloadStatus = "complete"
	DownloadFailed   DownloadStatus = "failed"

	DownloadStatusRunning  = DownloadRunning
	DownloadStatusComplete = DownloadComplete
	DownloadStatusFailed   = DownloadFailed
)

type DownloadRecord struct {
	URL          string
	TargetPath   string
	Status       DownloadStatus
	BytesWritten int64
	Error        string
	StartedAt    time.Time
	FinishedAt   time.Time
}

type DownloadManager struct {
	client *http.Client
}

func NewDownloadManager(client *http.Client) *DownloadManager {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &DownloadManager{client: client}
}

func (m *DownloadManager) Download(rawURL, targetPath string) (DownloadRecord, error) {
	return m.DownloadWithContext(context.Background(), rawURL, targetPath)
}

func (m *DownloadManager) DownloadWithContext(ctx context.Context, rawURL, targetPath string) (record DownloadRecord, resultErr error) {
	record = DownloadRecord{
		URL:        rawURL,
		TargetPath: targetPath,
		Status:     DownloadRunning,
		StartedAt:  time.Now(),
	}
	defer func() {
		record.FinishedAt = time.Now()
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		record.Status = DownloadFailed
		record.Error = err.Error()
		return record, err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		record.Status = DownloadFailed
		record.Error = err.Error()
		return record, err
	}
	bodyClosed := false
	defer func() {
		if !bodyClosed {
			_ = resp.Body.Close()
		}
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		resultErr = fmt.Errorf("download failed: HTTP %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
		record.Status = DownloadFailed
		record.Error = resultErr.Error()
		return record, resultErr
	}

	targetDir := filepath.Dir(targetPath)
	temp, err := os.CreateTemp(targetDir, "."+filepath.Base(targetPath)+".*.tmp")
	if err != nil {
		record.Status = DownloadFailed
		record.Error = err.Error()
		return record, err
	}
	tempPath := temp.Name()
	tempClosed := false
	committed := false
	defer func() {
		if !tempClosed {
			_ = temp.Close()
		}
		if !committed {
			_ = os.Remove(tempPath)
		}
	}()

	written, err := io.Copy(temp, resp.Body)
	record.BytesWritten = written
	if err != nil {
		record.Status = DownloadFailed
		record.Error = err.Error()
		return record, err
	}
	if err := ctx.Err(); err != nil {
		record.Status = DownloadFailed
		record.Error = err.Error()
		return record, err
	}
	bodyClosed = true
	if err := resp.Body.Close(); err != nil {
		record.Status = DownloadFailed
		record.Error = err.Error()
		return record, err
	}
	if err := temp.Close(); err != nil {
		tempClosed = true
		record.Status = DownloadFailed
		record.Error = err.Error()
		return record, err
	}
	tempClosed = true
	if err := os.Rename(tempPath, targetPath); err != nil {
		record.Status = DownloadFailed
		record.Error = err.Error()
		return record, err
	}
	committed = true
	record.Status = DownloadComplete
	return record, nil
}
