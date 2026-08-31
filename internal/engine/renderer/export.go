package renderer

import "sync"

// WriteEvent exports writeEvent for use by external test packages.
var WriteEvent = (*Child).writeEvent

// R exports r field for use by external test packages.
func (c *Child) R() interface{} { return c.r }

// W exports w field for use by external test packages.
func (c *Child) W() interface{} { return c.w }

// Session exports session field for use by external test packages.
func (c *Child) Session() interface{} { return c.session }

// Mu exports mu field for use by external test packages.
func (t *Tab) Mu() *sync.Mutex { return &t.mu }

// NextURL exports nextURL field for use by external test packages.
func (t *Tab) NextURL() string { return t.nextURL }
