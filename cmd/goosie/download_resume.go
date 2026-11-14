package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/vyquocvu/goosie/internal/download"
	"github.com/vyquocvu/goosie/internal/net"
)

var errDownloadCanceled = errors.New("goosie: download canceled")

type resumeDownloadPlan struct {
	mgr        *download.Manager
	client     net.HTTP
	itemID     string
	url        string
	tempPath   string
	received   int64
	targetPath string
	dir        string
	name       string
}

func (p *resumeDownloadPlan) register() {
	if p == nil || p.mgr == nil {
		return
	}
	p.mgr.SetTemp(p.itemID, p.tempPath, p.received)
	p.mgr.SetResume(p.itemID, p.run)
}

func (p *resumeDownloadPlan) run() {
	item := p.mgr.Get(p.itemID)
	if item == nil || (item.Status != download.StatusPaused && item.Status != download.StatusActive) {
		return
	}
	if item.TempPath != "" {
		p.tempPath = item.TempPath
	}
	if item.DownloadedBytes > 0 {
		p.received = item.DownloadedBytes
	}
	ranged, ok := p.client.(net.RangeStreamingHTTP)
	if !ok {
		p.mgr.Fail(p.itemID, errors.New("goosie: download client cannot continue a partial transfer"))
		return
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	res, err := ranged.GetStreamRange(ctx, p.url, p.received)
	if err != nil {
		p.mgr.Fail(p.itemID, err)
		return
	}
	defer res.Close()

	switch res.StatusCode {
	case http.StatusPartialContent:
	case http.StatusRequestedRangeNotSatisfiable:
		sum, hashErr := download.HashFile(p.tempPath)
		if hashErr != nil {
			p.mgr.Fail(p.itemID, hashErr)
			return
		}
		p.finish(p.received, sum)
		return
	case http.StatusOK:
		p.restart(ctx, cancel, res)
		return
	default:
		p.mgr.Fail(p.itemID, fmt.Errorf("goosie: resume %s: server returned %d instead of 206", net.RedactURL(p.url), res.StatusCode))
		return
	}

	p.mgr.SetCancel(p.itemID, func() {
		cancel(errDownloadCanceled)
		res.Cancel()
	})
	p.mgr.SetPause(p.itemID, func() {
		cancel(download.ErrPaused)
		res.Cancel()
	})

	total, sum, err := download.AppendToTemp(ctx, p.tempPath, res.Body, p.progressReporter())
	if errors.Is(err, download.ErrPaused) {
		p.received = total
		p.register()
		return
	}
	if err != nil {
		p.mgr.Fail(p.itemID, err)
		return
	}
	p.finish(total, sum)
}

func (p *resumeDownloadPlan) restart(ctx context.Context, cancel context.CancelCauseFunc, res *net.StreamResponse) {
	_ = os.Remove(p.tempPath)
	p.tempPath = ""
	p.received = 0
	p.mgr.Restart(p.itemID, res.ContentLength)

	p.mgr.SetCancel(p.itemID, func() {
		cancel(errDownloadCanceled)
		res.Cancel()
	})
	p.mgr.SetPause(p.itemID, func() {
		cancel(download.ErrPaused)
		res.Cancel()
	})

	var result download.StreamResult
	var err error
	if p.targetPath != "" {
		result, err = download.SaveStreamToPausable(ctx, p.targetPath, res.Body, p.progressReporter())
	} else {
		result, err = download.SaveStreamPausable(ctx, p.dir, p.name, res.Body, p.progressReporter())
	}
	if errors.Is(err, download.ErrPaused) {
		p.tempPath = result.TempPath
		p.received = result.Bytes
		p.register()
		return
	}
	if err != nil {
		p.mgr.Fail(p.itemID, err)
		return
	}
	p.mgr.UpdateProgress(p.itemID, result.Bytes)
	p.mgr.CompleteWithDigest(p.itemID, result.Path, result.Digest.String())
	if it := p.mgr.Get(p.itemID); it == nil || it.Status != download.StatusCompleted {
		_ = os.Remove(result.Path)
	}
}

func (p *resumeDownloadPlan) progressReporter() func(int64) {
	lastBytes := p.received
	lastAt := time.Now()
	return func(downloaded int64) {
		if downloaded-lastBytes >= 512*1024 || time.Since(lastAt) >= 250*time.Millisecond {
			p.mgr.UpdateProgress(p.itemID, downloaded)
			lastBytes = downloaded
			lastAt = time.Now()
		}
	}
}

func (p *resumeDownloadPlan) finish(total int64, sum download.Digest) {
	var finalPath string
	var err error
	if p.targetPath != "" {
		err = download.FinalizeTempTo(p.tempPath, p.targetPath)
		finalPath = p.targetPath
	} else {
		finalPath, err = download.FinalizeTemp(p.tempPath, p.dir, p.name)
	}
	if err != nil {
		p.mgr.Fail(p.itemID, err)
		return
	}
	p.mgr.UpdateProgress(p.itemID, total)
	p.mgr.CompleteWithDigest(p.itemID, finalPath, sum.String())
	if it := p.mgr.Get(p.itemID); it == nil || it.Status != download.StatusCompleted {
		_ = os.Remove(finalPath)
	}
}
