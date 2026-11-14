package js

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dop251/goja"
)

const MaxStorageBytes = 5 << 20

type storageObj struct {
	mu       sync.RWMutex
	data     map[string]string
	order    []string
	used     int64
	onChange func()
}

func newStorageObj() *storageObj {
	return &storageObj{
		data:  make(map[string]string),
		order: make([]string, 0),
	}
}

type OriginStorageRegistry struct {
	mu           sync.RWMutex
	localStores  map[string]*storageObj
	sessionStore map[string]*storageObj
	baseDir      string
	incognito    bool
}

func NewOriginStorageRegistry(baseDir string, incognito bool) *OriginStorageRegistry {
	return &OriginStorageRegistry{
		localStores:  make(map[string]*storageObj),
		sessionStore: make(map[string]*storageObj),
		baseDir:      baseDir,
		incognito:    incognito,
	}
}

func (r *OriginStorageRegistry) GetLocalStorage(origin string) *storageObj {
	if origin == "" {
		origin = "null"
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if s, ok := r.localStores[origin]; ok {
		return s
	}

	s := newStorageObj()
	if r.baseDir != "" && !r.incognito && origin != "null" {
		r.loadOriginLocked(origin, s)
		baseDir := r.baseDir
		s.onChange = func() {
			saveOrigin(baseDir, origin, s)
		}
	}
	r.localStores[origin] = s
	return s
}

func (r *OriginStorageRegistry) GetSessionStorage(sessionID, origin string) *storageObj {
	if origin == "" {
		origin = "null"
	}
	key := sessionID + "\x00" + origin
	r.mu.Lock()
	defer r.mu.Unlock()

	if s, ok := r.sessionStore[key]; ok {
		return s
	}
	s := newStorageObj()
	r.sessionStore[key] = s
	return s
}

func (r *OriginStorageRegistry) loadOriginLocked(origin string, s *storageObj) {
	targetFile := filepath.Join(r.baseDir, escapeOrigin(origin)+".json")
	data, err := os.ReadFile(targetFile)
	if err != nil {
		return
	}
	var loaded map[string]string
	if err := json.Unmarshal(data, &loaded); err != nil {
		return
	}
	s.data = loaded
	s.order = make([]string, 0, len(loaded))
	s.used = 0
	for k, v := range loaded {
		s.order = append(s.order, k)
		s.used += int64(len(k) + len(v))
	}
}

func saveOrigin(baseDir, origin string, s *storageObj) {
	if baseDir == "" || origin == "" || origin == "null" {
		return
	}
	s.mu.RLock()
	dataCopy := make(map[string]string, len(s.data))
	for k, v := range s.data {
		dataCopy[k] = v
	}
	s.mu.RUnlock()

	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return
	}
	encoded, err := json.MarshalIndent(dataCopy, "", "  ")
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(baseDir, ".storage-*")
	if err != nil {
		return
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(encoded); err != nil {
		_ = tmp.Close()
		return
	}
	if err := tmp.Close(); err != nil {
		return
	}
	targetFile := filepath.Join(baseDir, escapeOrigin(origin)+".json")
	_ = os.Rename(tmp.Name(), targetFile)
}

func escapeOrigin(origin string) string {
	var b strings.Builder
	for _, r := range origin {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func OriginOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "null"
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" && scheme != "file" {
		return "null"
	}
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else if scheme == "http" {
			port = "80"
		}
	}
	if port != "" {
		return scheme + "://" + host + ":" + port
	}
	return scheme + "://" + host
}

func (r *Runtime) setupStorage() {
	origin := OriginOf(r.opts.URL)
	var localStore *storageObj
	var sessionStore *storageObj

	if r.opts.Storage != nil {
		localStore = r.opts.Storage.GetLocalStorage(origin)
		sessionStore = r.opts.Storage.GetSessionStorage(r.opts.SessionID, origin)
	} else {
		localStore = newStorageObj()
		sessionStore = newStorageObj()
	}

	r.installStorage("localStorage", localStore)
	r.installStorage("sessionStorage", sessionStore)
}

func (r *Runtime) installStorage(name string, storage *storageObj) {
	obj := r.vm.NewObject()

	_ = obj.Set("getItem", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Null()
		}
		key := call.Arguments[0].String()
		storage.mu.RLock()
		val, ok := storage.data[key]
		storage.mu.RUnlock()
		if !ok {
			return goja.Null()
		}
		return r.vm.ToValue(val)
	})

	_ = obj.Set("setItem", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		key := call.Arguments[0].String()
		value := call.Arguments[1].String()

		storage.mu.Lock()
		old := 0
		_, exists := storage.data[key]
		if exists {
			old = len(key) + len(storage.data[key])
		}
		if storage.used-int64(old)+int64(len(key)+len(value)) > MaxStorageBytes {
			storage.mu.Unlock()
			panic(quotaExceededError(r))
		}
		if !exists {
			storage.order = append(storage.order, key)
		}
		storage.data[key] = value
		storage.used = storage.used - int64(old) + int64(len(key)+len(value))
		cb := storage.onChange
		storage.mu.Unlock()

		if cb != nil {
			cb()
		}

		return goja.Undefined()
	})

	_ = obj.Set("removeItem", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		key := call.Arguments[0].String()

		storage.mu.Lock()
		changed := false
		if prev, exists := storage.data[key]; exists {
			storage.used -= int64(len(key) + len(prev))
			delete(storage.data, key)
			for i, k := range storage.order {
				if k == key {
					storage.order = append(storage.order[:i], storage.order[i+1:]...)
					break
				}
			}
			changed = true
		}
		cb := storage.onChange
		storage.mu.Unlock()

		if changed && cb != nil {
			cb()
		}

		return goja.Undefined()
	})

	_ = obj.Set("clear", func(call goja.FunctionCall) goja.Value {
		storage.mu.Lock()
		storage.data = make(map[string]string)
		storage.order = make([]string, 0)
		storage.used = 0
		cb := storage.onChange
		storage.mu.Unlock()

		if cb != nil {
			cb()
		}

		return goja.Undefined()
	})

	_ = obj.Set("key", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Null()
		}
		index := int(call.Arguments[0].ToInteger())

		storage.mu.RLock()
		if index < 0 || index >= len(storage.order) {
			storage.mu.RUnlock()
			return goja.Null()
		}
		key := storage.order[index]
		storage.mu.RUnlock()

		return r.vm.ToValue(key)
	})

	_ = obj.DefineAccessorProperty("length",
		r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			storage.mu.RLock()
			n := len(storage.data)
			storage.mu.RUnlock()
			return r.vm.ToValue(n)
		}),
		goja.Undefined(), goja.FLAG_FALSE, goja.FLAG_TRUE)

	_ = r.vm.Set(name, obj)

	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set(name, obj)
		}
	}
}

func quotaExceededError(r *Runtime) goja.Value {
	e := r.vm.NewObject()
	_ = e.Set("name", "QuotaExceededError")
	_ = e.Set("message", "Storage quota exceeded")
	return e
}

type StorageEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (r *OriginStorageRegistry) DumpLocalStorage(origin string) []StorageEntry {
	if r == nil {
		return nil
	}
	if origin == "" {
		origin = "null"
	}
	r.mu.Lock()
	s, ok := r.localStores[origin]
	if !ok {
		s = newStorageObj()
		if r.baseDir != "" && !r.incognito && origin != "null" {
			r.loadOriginLocked(origin, s)
		}
		r.localStores[origin] = s
	}
	r.mu.Unlock()

	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]StorageEntry, 0, len(s.order))
	for _, k := range s.order {
		if v, ok := s.data[k]; ok {
			out = append(out, StorageEntry{Key: k, Value: v})
		}
	}
	return out
}

func (r *OriginStorageRegistry) ClearLocalStorage(origin string) {
	if r == nil {
		return
	}
	if origin == "" {
		origin = "null"
	}
	r.mu.Lock()
	s, ok := r.localStores[origin]
	r.mu.Unlock()
	if !ok {
		return
	}
	s.mu.Lock()
	s.data = make(map[string]string)
	s.order = nil
	s.used = 0
	onChange := s.onChange
	s.mu.Unlock()
	if onChange != nil {
		onChange()
	}
}

func (r *OriginStorageRegistry) ClearAllLocalStorage() (origins, files int, err error) {
	if r == nil {
		return 0, 0, nil
	}
	r.mu.Lock()
	stores := make([]*storageObj, 0, len(r.localStores))
	for _, s := range r.localStores {
		stores = append(stores, s)
	}
	baseDir := r.baseDir
	incognito := r.incognito
	r.mu.Unlock()

	for _, s := range stores {
		s.mu.Lock()
		s.data = make(map[string]string)
		s.order = nil
		s.used = 0
		s.mu.Unlock()
	}
	origins = len(stores)

	if baseDir == "" || incognito {
		return origins, 0, nil
	}
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return origins, 0, nil
		}
		return origins, 0, err
	}
	var problems []error
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		if rmErr := os.Remove(filepath.Join(baseDir, e.Name())); rmErr != nil {
			if !os.IsNotExist(rmErr) {
				problems = append(problems, rmErr)
			}
			continue
		}
		files++
	}
	return origins, files, errors.Join(problems...)
}

func (r *OriginStorageRegistry) SetLocalStorageItem(origin, key, val string) {
	if r == nil {
		return
	}
	s := r.GetLocalStorage(origin)
	s.mu.Lock()
	if _, exists := s.data[key]; !exists {
		s.order = append(s.order, key)
	}
	s.data[key] = val
	onChange := s.onChange
	s.mu.Unlock()
	if onChange != nil {
		onChange()
	}
}

func (r *OriginStorageRegistry) RemoveLocalStorageItem(origin, key string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	s, ok := r.localStores[origin]
	r.mu.Unlock()
	if !ok {
		return
	}
	s.mu.Lock()
	if _, exists := s.data[key]; exists {
		delete(s.data, key)
		for i, k := range s.order {
			if k == key {
				s.order = append(s.order[:i], s.order[i+1:]...)
				break
			}
		}
	}
	onChange := s.onChange
	s.mu.Unlock()
	if onChange != nil {
		onChange()
	}
}
