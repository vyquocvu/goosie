package dom

import "github.com/vyquocvu/goosie/internal/dom/atom"

type MutationKind uint8

const (
	MutationInsert MutationKind = iota
	MutationRemove
	MutationReplace
	MutationSetText
	MutationSetAttribute
)

type Mutation struct {
	Kind      MutationKind
	Target    NodeID
	Parent    NodeID
	Reference NodeID
	Attribute atom.Atom
	Value     string
	NewNode   NodeID
}

func (s *Store) ApplyMutation(m Mutation) error {
	var err error
	switch m.Kind {
	case MutationInsert:
		err = s.InsertBefore(m.Parent, m.Target, m.Reference)
	case MutationRemove:
		err = s.RemoveChild(m.Parent, m.Target)
	case MutationReplace:
		err = s.Replace(m.Target, m.NewNode)
	case MutationSetText:
		err = s.SetText(m.Target, m.Value)
	case MutationSetAttribute:
		err = s.setAttribute(m.Target, m.Attribute, m.Value)
	default:
		return ErrInvalidNodeID
	}
	if err != nil {
		return err
	}
	if m.Target != NodeNone {
		_ = s.SetFlag(m.Target, NodeFlagDirty)
	}
	if m.Parent != NodeNone {
		_ = s.SetFlag(m.Parent, NodeFlagDirty)
	}
	return nil
}

func (s *Store) setAttribute(id NodeID, name atom.Atom, value string) error {
	attrs, err := s.Attrs(id)
	if err != nil {
		return err
	}
	updated := make([]Attr, 0, len(attrs)+1)
	found := false
	for _, attr := range attrs {
		if attr.Name == name {
			found = true
			if value != "" {
				updated = append(updated, Attr{Name: name, Value: atom.Intern(value)})
			}
			continue
		}
		updated = append(updated, attr)
	}
	if !found && value != "" {
		updated = append(updated, Attr{Name: name, Value: atom.Intern(value)})
	}
	return s.SetAttrs(id, updated)
}
