package js

import "github.com/dop251/goja"

// VM exports vm field for use by external test packages.
func (r *Runtime) VM() *goja.Runtime { return r.vm }

// SetVM sets vm field for use by external test packages.
func (r *Runtime) SetVM(vm *goja.Runtime) { r.vm = vm }

// HTMLCache exports htmlCache field for use by external test packages.
func (r *Runtime) HTMLCache() string { return r.htmlCache }

// SetHTMLCache sets htmlCache field for use by external test packages.
func (r *Runtime) SetHTMLCache(cache string) { r.htmlCache = cache }

// EnqueueTask exports enqueueTask field for use by external test packages.
func (r *Runtime) EnqueueTask(fn func()) { r.enqueueTask(fn) }

// SetEnqueueTask sets enqueueTask field for use by external test packages.
func (r *Runtime) SetEnqueueTask(fn func(func())) { r.enqueueTask = fn }

// HasEnqueueTask reports whether the enqueueTask callback is wired.
func (r *Runtime) HasEnqueueTask() bool { return r.enqueueTask != nil }

// MaxPermissionDecisions exports maxPermissionDecisions for use by external test packages.
const MaxPermissionDecisions = maxPermissionDecisions

// Timers exports timers field for use by external test packages.
func (r *Runtime) Timers() map[int]*Timer { return r.timers }

// Fetcher exports fetcher field for use by external test packages.
func (r *Runtime) Fetcher() HTTPFetcher { return r.fetcher }

// CheckFileFetchAccess exports checkFileFetchAccess for use by external test packages.
var CheckFileFetchAccess = checkFileFetchAccess

// DrainTasks exports drainTasks for use by external test packages.
func (s *Session) DrainTasks() { s.drainTasks() }
