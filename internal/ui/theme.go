package ui

import (
	"image/color"

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
	headless  bool
	listeners []func(ThemeType)
}

// NewThemeManager creates a new ThemeManager
func NewThemeManager(app fyne.App, headless ...bool) *ThemeManager {
	h := len(headless) > 0 && headless[0]
	tm := &ThemeManager{
		app:      app,
		current:  ThemeSystem,
		headless: h,
	}
	if !h {
		tm.load()
	} else {
		tm.apply()
	}
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

type browserTheme struct {
	variant fyne.ThemeVariant
}

func (t *browserTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	if t.variant == theme.VariantDark {
		switch name {
		case theme.ColorNameBackground:
			return color.RGBA{R: 32, G: 33, B: 36, A: 255} // Chrome Dark Background
		case theme.ColorNameInputBackground:
			return color.RGBA{R: 41, G: 42, B: 45, A: 255} // Chrome Dark Input
		case theme.ColorNameButton:
			return color.RGBA{R: 48, G: 49, B: 52, A: 255}
		case theme.ColorNameHover:
			return color.RGBA{R: 60, G: 64, B: 67, A: 255}
		case theme.ColorNamePressed:
			return color.RGBA{R: 80, G: 84, B: 88, A: 255}
		case theme.ColorNamePrimary:
			return color.RGBA{R: 138, G: 180, B: 248, A: 255} // Accent Blue
		case theme.ColorNameForeground:
			return color.RGBA{R: 232, G: 234, B: 237, A: 255}
		case theme.ColorNameFocus:
			return color.RGBA{R: 138, G: 180, B: 248, A: 255}
		case theme.ColorNameSeparator:
			return color.RGBA{R: 60, G: 64, B: 67, A: 255}
		case theme.ColorNameScrollBar:
			return color.RGBA{R: 128, G: 128, B: 128, A: 128}
		}
	} else {
		// Light variant
		switch name {
		case theme.ColorNameBackground:
			return color.RGBA{R: 241, G: 243, B: 244, A: 255} // Chrome Light Background
		case theme.ColorNameInputBackground:
			return color.RGBA{R: 255, G: 255, B: 255, A: 255}
		case theme.ColorNameButton:
			return color.RGBA{R: 241, G: 243, B: 244, A: 255} // Flat opaque buttons
		case theme.ColorNameHover:
			return color.RGBA{R: 0, G: 0, B: 0, A: 20}
		case theme.ColorNamePressed:
			return color.RGBA{R: 0, G: 0, B: 0, A: 40}
		case theme.ColorNamePrimary:
			return color.RGBA{R: 26, G: 115, B: 232, A: 255}
		case theme.ColorNameForeground:
			return color.RGBA{R: 60, G: 64, B: 67, A: 255}
		case theme.ColorNameFocus:
			return color.RGBA{R: 26, G: 115, B: 232, A: 255}
		case theme.ColorNameSeparator:
			return color.RGBA{R: 218, G: 220, B: 224, A: 255}
		case theme.ColorNameScrollBar:
			return color.RGBA{R: 0, G: 0, B: 0, A: 80}
		}
	}
	return theme.DefaultTheme().Color(name, t.variant)
}

func (t *browserTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (t *browserTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (t *browserTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 4
	case theme.SizeNameInputRadius:
		return 16 // Rounded capsule look
	case theme.SizeNameScrollBar:
		return 8
	}
	return theme.DefaultTheme().Size(name)
}

func (tm *ThemeManager) apply() {
	var variant fyne.ThemeVariant
	switch tm.current {
	case ThemeLight:
		variant = theme.VariantLight
	case ThemeDark:
		variant = theme.VariantDark
	case ThemeSystem:
		if tm.headless {
			variant = theme.VariantLight
		} else {
			variant = tm.app.Settings().ThemeVariant()
		}
	}
	if tm.app != nil {
		tm.app.Settings().SetTheme(&browserTheme{variant: variant})
	}
}

// save persists the theme preference
func (tm *ThemeManager) save() {
	if tm.headless {
		return
	}
	tm.app.Preferences().SetInt("theme_preference", int(tm.current))
}

// load retrieves the persisted theme preference
func (tm *ThemeManager) load() {
	if tm.headless {
		return
	}
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

