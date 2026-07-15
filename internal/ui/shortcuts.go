package ui

type ShortcutRegistry struct {
	commands map[string]func()
}

func NewShortcutRegistry() *ShortcutRegistry {
	return &ShortcutRegistry{commands: map[string]func(){}}
}

func (r *ShortcutRegistry) Register(name string, command func()) {
	r.commands[name] = command
}

func (r *ShortcutRegistry) Dispatch(name string) bool {
	command, ok := r.commands[name]
	if !ok {
		return false
	}
	command()
	return true
}

// devToolsShortcutName is the shortcut name for toggling dev tools.
const devToolsShortcutName = "toggle-dev-tools"
