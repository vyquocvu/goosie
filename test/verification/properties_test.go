package verification_test

import (
	"fmt"
	"math/rand"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/quick"
	"unicode"

	"github.com/vyquocvu/goosie/internal/download"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/preferences"
	"github.com/vyquocvu/goosie/internal/session"
)

func checkProperty(t *testing.T, count int, property any) {
	t.Helper()
	const seed = 20261117
	t.Logf("seed=%d cases=%d", seed, count)
	if err := quick.Check(property, &quick.Config{MaxCount: count, Rand: rand.New(rand.NewSource(seed))}); err != nil {
		t.Fatal(err)
	}
}

func TestPropertyRectCanonicalizationIdempotent(t *testing.T) {
	checkProperty(t, 1000, func(coords [4]int16) bool {
		r := frame.Rect4(int32(coords[0]), int32(coords[1]), int32(coords[2]), int32(coords[3]))
		c := r.Canon()
		return c == c.Canon() && c.IsNormal() &&
			c.X0 == min(r.X0, r.X1) && c.X1 == max(r.X0, r.X1) &&
			c.Y0 == min(r.Y0, r.Y1) && c.Y1 == max(r.Y0, r.Y1)
	})
}

func TestPropertyRectTranslationRoundTrip(t *testing.T) {
	checkProperty(t, 1000, func(coords [6]int16) bool {
		r := frame.Rect4(int32(coords[0]), int32(coords[1]), int32(coords[2]), int32(coords[3]))
		dx, dy := int32(coords[4]), int32(coords[5])
		moved := r.Translate(dx, dy)
		return moved.Translate(-dx, -dy) == r && moved.Size() == r.Size() && moved.Empty() == r.Empty() &&
			moved.X0 == r.X0+dx && moved.Y0 == r.Y0+dy && moved.X1 == r.X1+dx && moved.Y1 == r.Y1+dy
	})
}

func TestPropertyIntersectionEquivalentToPixelSets(t *testing.T) {
	nonempty := 0
	checkProperty(t, 1000, func(coords [8]int8) bool {
		for i := range coords {
			coords[i] %= 8
		}
		a := frame.Rect4(int32(coords[0]), int32(coords[1]), int32(coords[2]), int32(coords[3])).Canon()
		b := frame.Rect4(int32(coords[4]), int32(coords[5]), int32(coords[6]), int32(coords[7])).Canon()
		pixels := make(map[frame.Point]bool)
		for x := a.X0; x < a.X1; x++ {
			for y := a.Y0; y < a.Y1; y++ {
				pixels[frame.Point{X: x, Y: y}] = true
			}
		}
		want := make(map[frame.Point]bool)
		for x := b.X0; x < b.X1; x++ {
			for y := b.Y0; y < b.Y1; y++ {
				p := frame.Point{X: x, Y: y}
				if pixels[p] {
					want[p] = true
				}
			}
		}
		intersection := a.Intersection(b)
		got := make(map[frame.Point]bool)
		for x := intersection.X0; x < intersection.X1; x++ {
			for y := intersection.Y0; y < intersection.Y1; y++ {
				got[frame.Point{X: x, Y: y}] = true
			}
		}
		if len(want) > 0 {
			nonempty++
		}
		return reflect.DeepEqual(got, want) && a.Intersects(b) == (len(want) > 0)
	})
	if nonempty == 0 || nonempty == 1000 {
		t.Fatalf("vacuous intersection population: %d/1000 nonempty", nonempty)
	}
	t.Logf("nonempty=%d empty=%d", nonempty, 1000-nonempty)
}

func normalizedLines(value string) string {
	var result strings.Builder
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '\r':
			if i+1 < len(value) && value[i+1] == '\n' {
				i++
			}
			result.WriteString("\r\n")
		case '\n':
			result.WriteString("\r\n")
		default:
			result.WriteByte(value[i])
		}
	}
	return result.String()
}

func TestPropertyFormURLRoundTrip(t *testing.T) {
	checkProperty(t, 1000, func(name, value, second string) bool {
		name += "&=+\r\n"
		value += "\r\n\r\n\n\r &=+%"
		data := []engine.FormData{{Name: name, Value: value}, {Name: name, Value: second}, {Name: "", Value: ""}}
		got, err := url.ParseQuery(engine.EncodeFormURLEncoded(data))
		want := url.Values{normalizedLines(name): {normalizedLines(value), normalizedLines(second)}, "": {""}}
		return err == nil && reflect.DeepEqual(got, want)
	})
}

func TestPropertySearchQueryRoundTrip(t *testing.T) {
	checkProperty(t, 1000, func(query string) bool {
		query += " &=+#%\r\n"
		raw, err := preferences.SearchURL("https://search.test/find?fixed=keep&q={query}#result", query)
		if err != nil {
			return false
		}
		u, err := url.Parse(raw)
		return err == nil && u.Scheme == "https" && u.Host == "search.test" &&
			u.Path == "/find" && u.Fragment == "result" && len(u.Query()) == 2 &&
			u.Query().Get("fixed") == "keep" && u.Query().Get("q") == query
	})
}

func TestPropertyDownloadFilenameIsSingleSafeComponent(t *testing.T) {
	checkProperty(t, 1000, func(name, rawURL string) bool {
		for _, disposition := range []string{"", "attachment; filename=\"../\\" + name + "\"", name} {
			got := download.FileName(disposition, rawURL)
			if got == "" || got == "." || got == ".." || strings.ContainsAny(got, "/\\") || filepath.Base(got) != got {
				return false
			}
			for _, r := range got {
				if unicode.IsControl(r) {
					return false
				}
			}
		}
		return true
	})
}

func TestPropertyViewportEquivalentToRationalBudget(t *testing.T) {
	accepted := 0
	checkProperty(t, 4000, func(width, height uint16, numerator int8) bool {
		w, h, n := int64(width), int64(height), int64(numerator%37)
		rw, rh := (w*n+2)/4, (h*n+2)/4
		want := w > 0 && h > 0 && w <= engine.MaxGeometry && h <= engine.MaxGeometry && n > 0 && n <= 32 &&
			w*n <= engine.MaxDeviceDimension*4 && h*n <= engine.MaxDeviceDimension*4 &&
			rw >= 1 && rh >= 1 && w*h*n*n <= engine.MaxDevicePixels*16 && rw*rh <= engine.MaxDevicePixels
		got := engine.ValidateViewport(int(width), int(height), float64(n)/4) == nil
		if got {
			accepted++
		}
		return got == want
	})
	if accepted == 0 || accepted == 4000 {
		t.Fatalf("vacuous admission population: %d/4000 accepted", accepted)
	}
	t.Logf("accepted=%d rejected=%d", accepted, 4000-accepted)
}

func TestPropertyPreferencesPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "preferences.json")
	checkProperty(t, 128, func(query string, newTab, ask bool) bool {
		mode := preferences.StartupRestore
		if newTab {
			mode = preferences.StartupNewTab
		}
		want := preferences.Settings{
			SearchURLTemplate: "https://search.test/?fixed=" + url.QueryEscape(query) + "&q={query}",
			DownloadDir:       dir, StartupMode: mode, AskDownloadLocation: ask,
		}
		if err := preferences.Save(path, want); err != nil {
			t.Log(err)
			return false
		}
		got, err := preferences.Load(path)
		return err == nil && got == want
	})
}

func TestPropertySessionNormalizationIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	checkProperty(t, 128, func(tabCount, historyCount uint8, active, index int8, title string) bool {
		state := session.State{Active: int(active)}
		for i := 0; i < int(tabCount%64); i++ {
			tab := session.TabState{URL: "https://page.test/original", Title: title, HistoryIndex: int(index)}
			for j := 0; j < int(historyCount%128); j++ {
				tab.History = append(tab.History, fmt.Sprintf("https://page.test/%d/%d", i, j))
			}
			state.Tabs = append(state.Tabs, tab)
		}
		if err := session.Save(path, state); err != nil {
			t.Log(err)
			return false
		}
		first, err := session.Load(path)
		if err != nil || len(first.Tabs) > session.MaxTabs || first.Active < 0 || first.Active >= max(1, len(first.Tabs)) {
			return false
		}
		for _, tab := range first.Tabs {
			if len(tab.History) > session.MaxHistoryPerTab || tab.HistoryIndex < -1 || tab.HistoryIndex >= len(tab.History) {
				return false
			}
			if tab.HistoryIndex >= 0 && tab.URL != tab.History[tab.HistoryIndex] {
				return false
			}
		}
		if err := session.Save(path, first); err != nil {
			return false
		}
		second, err := session.Load(path)
		return err == nil && reflect.DeepEqual(first, second)
	})
}
