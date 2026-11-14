package js

import (
	"github.com/vyquocvu/goosie/internal/dom"
)

type MutationBudget interface {
	Reserve(parent *dom.Node, children []*dom.Node) error
	Release(children []*dom.Node)
	IsLive(n *dom.Node) bool
}

type SubresourceBudget interface {
	Take(n int) bool
}

func (r *Runtime) SetMutationBudget(b MutationBudget) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mutBudget = b
}

func (r *Runtime) SetSubresourceBudget(b SubresourceBudget) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subBudget = b
}

func (r *Runtime) mutationBudget() MutationBudget {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.mutBudget
}

func (r *Runtime) subresourceBudget() SubresourceBudget {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.subBudget
}

func (r *Runtime) admitInsert(op string, parent *dom.Node, children []*dom.Node, commit func()) {
	if b := r.mutationBudget(); b != nil {
		if err := b.Reserve(parent, children); err != nil {
			r.throwQuota(op, err)
			return
		}
	}
	commit()
}

func (r *Runtime) releaseNodes(children []*dom.Node) {
	if b := r.mutationBudget(); b != nil {
		b.Release(children)
	}
}

func (r *Runtime) nodeIsLive(n *dom.Node) bool {
	if b := r.mutationBudget(); b != nil {
		return b.IsLive(n)
	}
	return false
}

func (r *Runtime) throwQuota(op string, err error) {
	e := r.vm.NewTypeError(op + ": " + err.Error())
	_ = e.Set("name", "QuotaExceededError")
	panic(e)
}
