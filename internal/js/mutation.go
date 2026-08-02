package js

type MutationKind uint8

const (
	MutationUnknown MutationKind = iota
	MutationBatch
	MutationInsert
	MutationRemove
	MutationReplace
	MutationSetText
	MutationSetAttribute
)

type DOMMutation struct {
	Kind        MutationKind
	Count       int
	TargetID    string
	ParentID   string
	ReferenceID string
	Attribute  string
	OldValue   string
	NewValue   string
}
