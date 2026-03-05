package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// ThemeType represents the theme preference
type ThemeType int

const (
	// ThemeSystem follows the operating system's theme
	ThemeSystem ThemeType = iota
	// ThemeLight forces light theme
	ThemeLight
	// ThemeDark forces dark theme
	ThemeDark
)

// String returns the string representation of the theme type
func (t ThemeType) String() string {
	switch t {
	case ThemeLight:
		return "Light"
	case ThemeDark:
		return "Dark"
	default:
		return "System"
	}
}

// ThemeManager handles theme state and application
type ThemeManager struct {
	app       fyne.App
	current   ThemeType
	listeners []func(ThemeType)
}

// NewThemeManager creates a new ThemeManager
func NewThemeManager(app fyne.App) *ThemeManager {
	tm := &ThemeManager{
		app:     app,
		current: ThemeSystem,
	}
	tm.load()
	return tm
}

// SetTheme sets the current theme preference
func (tm *ThemeManager) SetTheme(t ThemeType) {
	tm.current = t
	tm.apply()
	tm.save()
	for _, l := range tm.listeners {
		l(t)
	}
}

// apply applies the current theme to the Fyne application
func (tm *ThemeManager) apply() {
	switch tm.current {
	case ThemeLight:
		tm.app.Settings().SetTheme(theme.LightTheme())
	case ThemeDark:
		tm.app.Settings().SetTheme(theme.DarkTheme())
	case ThemeSystem:
		tm.app.Settings().SetTheme(theme.DefaultTheme())
	}
}

// save persists the theme preference
func (tm *ThemeManager) save() {
	tm.app.Preferences().SetInt("theme_preference", int(tm.current))
}

// load retrieves the persisted theme preference
func (tm *ThemeManager) load() {
	val := tm.app.Preferences().IntWithFallback("theme_preference", int(ThemeSystem))
	tm.current = ThemeType(val)
	tm.apply()
}

// Current returns the current theme preference
func (tm *ThemeManager) Current() ThemeType {
	return tm.current
}

// AddListener adds a listener for theme changes
func (tm *ThemeManager) AddListener(l func(ThemeType)) {
	tm.listeners = append(tm.listeners, l)
}
