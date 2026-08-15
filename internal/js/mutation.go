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

// needsFullReparse reports whether a mutation kind must fall back to the
// full DOM serialize + reparse path. The typed batch path fully handles
// set-text and set-attribute (the sink syncs the value into the render
// tree and invalidates), so those skip serialization. Everything else —
// structural edits and unclassified kinds — still needs the string
// callback because the typed sink cannot yet synthesize render subtrees.
func needsFullReparse(kind MutationKind) bool {
	switch kind {
	case MutationSetText, MutationSetAttribute:
		return false
	default:
		return true
	}
}
