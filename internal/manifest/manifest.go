package manifest

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

type DisplayMode string

const (
	DisplayBrowser    DisplayMode = "browser"
	DisplayStandalone DisplayMode = "standalone"
	DisplayMinimalUI  DisplayMode = "minimal-ui"
	DisplayFullscreen DisplayMode = "fullscreen"
)

type Icon struct {
	Src   string `json:"src"`
	Sizes string `json:"sizes,omitempty"`
	Type  string `json:"type,omitempty"`
}

type Manifest struct {
	Name            string      `json:"name"`
	ShortName       string      `json:"short_name,omitempty"`
	Description     string      `json:"description,omitempty"`
	StartURL        string      `json:"start_url,omitempty"`
	Display         DisplayMode `json:"display,omitempty"`
	Orientation     string      `json:"orientation,omitempty"`
	BackgroundColor string      `json:"background_color,omitempty"`
	ThemeColor      string      `json:"theme_color,omitempty"`
	Icons           []Icon      `json:"icons,omitempty"`
}

func (m *Manifest) EffectiveName() string {
	if m == nil {
		return ""
	}
	if strings.TrimSpace(m.Name) != "" {
		return strings.TrimSpace(m.Name)
	}
	return strings.TrimSpace(m.ShortName)
}

func (m *Manifest) IsStandalone() bool {
	if m == nil {
		return false
	}
	d := strings.ToLower(string(m.Display))
	return d == string(DisplayStandalone) || d == string(DisplayFullscreen)
}

func (m *Manifest) ResolveStartURL(baseURL string) string {
	if m == nil || m.StartURL == "" {
		return baseURL
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return m.StartURL
	}
	ref, err := url.Parse(m.StartURL)
	if err != nil {
		return m.StartURL
	}
	return base.ResolveReference(ref).String()
}

func Parse(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	m.Display = DisplayMode(strings.ToLower(string(m.Display)))
	switch m.Display {
	case DisplayStandalone, DisplayMinimalUI, DisplayFullscreen:
	default:
		m.Display = DisplayBrowser
	}
	return &m, nil
}

func ExtractManifestURL(htmlStr, baseURL string) string {
	re := regexp.MustCompile(`(?i)<link[^>]+(?:rel=["']?manifest["']?[^>]+href=["']([^"'>]+)["']|href=["']([^"'>]+)["'][^>]+rel=["']?manifest["']?)`)
	matches := re.FindStringSubmatch(htmlStr)
	href := ""
	if len(matches) > 1 && matches[1] != "" {
		href = matches[1]
	} else if len(matches) > 2 && matches[2] != "" {
		href = matches[2]
	}

	if href == "" {
		return ""
	}

	base, err := url.Parse(baseURL)
	if err != nil {
		return href
	}
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	return base.ResolveReference(ref).String()
}
