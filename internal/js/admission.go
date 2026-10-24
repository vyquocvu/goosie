package js

import (
	"github.com/vyquocvu/goosie/internal/dom"
)

// MutationBudget gates script-driven DOM insertions against the host document's
// admission caps. internal/js declares the contract so it never imports
// internal/engine (the archtest import graph forbids it); the engine supplies
// the concrete implementation and installs it with SetMutationBudget before any
// script runs. A nil budget leaves insertions ungated, which is what tests and
// non-engine hosts want.
type MutationBudget interface {
	// Reserve reports whether children may be inserted under parent without
	// breaching the document's node-count, depth, or attribute caps. It charges
	// the budget on success and returns a non-nil error otherwise, leaving the
	// document untouched. A child already live in the document (a move) must not
	// be charged again. Implementations are called from the runtime's single
	// mutating thread but must guard their own counter for the fetch/XHR
	// goroutines that share the process.
	Reserve(parent *dom.Node, children []*dom.Node) error
	// Release credits a subtree the caller has just detached from the document,
	// so a remove-then-append loop does not monotonically exhaust the budget.
	// Callers must pass only nodes that were live before detaching (see IsLive);
	// crediting a node that lived under a detached parent — never charged in the
	// first place — would under-count the document and let a later insertion
	// slip past the cap.
	Release(children []*dom.Node)
	// IsLive reports whether n is currently reachable from the budget's document
	// root. Removal paths consult it before detaching so Release credits exactly
	// the nodes that were charged, never nodes that sat under a detached parent.
	IsLive(n *dom.Node) bool
}

// SubresourceBudget is the shared byte total a document may spend on
// subresources. CSS, fonts and images already draw on it inside the engine;
// declaring it here lets fetch and XHR charge the same pool without importing
// internal/engine. A nil budget leaves the JS network APIs ungated.
type SubresourceBudget interface {
	// Take charges n bytes, reporting false when they do not fit. A caller whose
	// Take fails must abandon the payload rather than use it.
	Take(n int) bool
}

// SetMutationBudget installs the document admission budget that gates every
// script-driven DOM insertion. The engine calls it after SetDOM and before any
// script runs. Safe for concurrent use; a nil budget disables gating.
func (r *Runtime) SetMutationBudget(b MutationBudget) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mutBudget = b
}

// SetSubresourceBudget installs the shared byte budget that fetch and XHR charge
// their response bodies to. The engine calls it before scripts run so a page
// cannot pull unbounded bytes through the JS network APIs. nil disables gating.
func (r *Runtime) SetSubresourceBudget(b SubresourceBudget) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subBudget = b
}

// mutationBudget reads the installed budget under the runtime lock. Insertions
// run on the VM thread, where r.mu is not held (Run releases it before executing
// a program), so this never deadlocks.
func (r *Runtime) mutationBudget() MutationBudget {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.mutBudget
}

// subresourceBudget reads the installed byte budget under the runtime lock. The
// value is captured on the VM thread and carried onto the fetch/XHR goroutines,
// so those never touch r.mu off-thread.
func (r *Runtime) subresourceBudget() SubresourceBudget {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.subBudget
}

// admitInsert is the single fallible insertion path every script-driven DOM
// write funnels through. It reserves the insertion against the document's
// mutation budget BEFORE running commit, so a rejection leaves the DOM exactly
// as it was — no partial insert, and (for innerHTML/textContent) no cleared
// parent. When no budget is installed, commit runs ungated. A rejection throws a
// catchable QuotaExceededError rather than crashing the session.
func (r *Runtime) admitInsert(op string, parent *dom.Node, children []*dom.Node, commit func()) {
	if b := r.mutationBudget(); b != nil {
		if err := b.Reserve(parent, children); err != nil {
			r.throwQuota(op, err)
			return // unreachable: throwQuota panics
		}
	}
	commit()
}

// releaseNodes credits detached subtrees back to the mutation budget. It is
// called after a removal so the live count tracks the document and a
// remove-then-append loop does not monotonically exhaust the budget.
func (r *Runtime) releaseNodes(children []*dom.Node) {
	if b := r.mutationBudget(); b != nil {
		b.Release(children)
	}
}

// nodeIsLive reports whether n is reachable from the budget's document root. It
// is consulted BEFORE a removal detaches n, so releaseNodes credits only nodes
// that were actually charged. With no budget installed it returns false, which
// is harmless because releaseNodes is then a no-op.
func (r *Runtime) nodeIsLive(n *dom.Node) bool {
	if b := r.mutationBudget(); b != nil {
		return b.IsLive(n)
	}
	return false
}

// throwQuota panics with a catchable JS exception named QuotaExceededError, the
// DOMException a real browser raises when an operation exceeds a resource quota.
// goja recovers the panic at the script frame, so a page's try/catch sees it and
// the session keeps running; an uncaught one is swallowed by the engine's
// script runner like any other script error.
func (r *Runtime) throwQuota(op string, err error) {
	e := r.vm.NewTypeError(op + ": " + err.Error())
	_ = e.Set("name", "QuotaExceededError")
	panic(e)
}
