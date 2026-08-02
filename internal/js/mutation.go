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
	ParentID    string
	ReferenceID string
	Attribute   string
	OldValue    string
	NewValue    string
}

func mutationKindFromString(value string) MutationKind {
	switch value {
	case "insert":
		return MutationInsert
	case "remove":
		return MutationRemove
	case "replace":
		return MutationReplace
	case "set-text":
		return MutationSetText
	case "set-attribute":
		return MutationSetAttribute
	default:
		return MutationUnknown
	}
}
