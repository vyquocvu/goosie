package raster

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/raster"
)

var fontSlots = []frame.FontSlot{
	{},
	{Bold: true},
	{Italic: true},
	{Bold: true, Italic: true},
	{Family: frame.FontTimes},
	{Family: frame.FontTimes, Bold: true},
	{Family: frame.FontTimes, Italic: true},
	{Family: frame.FontTimes, Bold: true, Italic: true},
	{Family: frame.FontArial},
	{Family: frame.FontArial, Bold: true},
	{Family: frame.FontCourier},
	{Family: frame.FontCourier, Italic: true},
	{Family: frame.FontGeorgia},
	{Family: frame.FontGeorgia, Bold: true, Italic: true},
	{Family: frame.FontVerdana},
	{Family: frame.FontVerdana, Bold: true},
}

func TestFontsWithoutSystemFontsStillDraw(t *testing.T) {
	f, err := raster.NewEmbeddedFonts()
	if err != nil {
		t.Fatalf("NewEmbeddedFonts: %v", err)
	}
	for _, slot := range fontSlots {
		g := f.Glyph(16, 'A', slot)
		if !g.Ok || g.Mask == nil {
			t.Fatalf("slot %+v draws no glyph", slot)
		}
		if f.GlyphAdvance(16, 'A', slot) <= 0 {
			t.Errorf("slot %+v reports no advance", slot)
		}
	}
}

func TestSystemFontFlagFallsBackPerSlot(t *testing.T) {
	t.Setenv("GOOSIE_SYSTEM_FONTS", "off")
	f, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("NewFonts: %v", err)
	}
	for _, slot := range fontSlots {
		want := f.Glyph(16, 'A', frame.FontSlot{Bold: slot.Bold})
		got := f.Glyph(16, 'A', slot)
		if want.Mask == nil || got.Mask == nil {
			t.Fatalf("slot %+v: no mask", slot)
		}
		if !bytes.Equal(got.Mask.Pix, want.Mask.Pix) || got.Bounds != want.Bounds {
			t.Errorf("slot %+v did not fall back to the embedded face", slot)
		}
	}
}

func TestSlotsSharingAFaceShareItsGlyphs(t *testing.T) {
	t.Setenv("GOOSIE_SYSTEM_FONTS", "off")
	f, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("NewFonts: %v", err)
	}
	times := f.Glyph(16, 'a', frame.FontSlot{Family: frame.FontTimes})
	georgia := f.Glyph(16, 'a', frame.FontSlot{Family: frame.FontGeorgia})
	if times.Mask == nil || georgia.Mask == nil {
		t.Fatal("no glyph mask")
	}
	if times.Mask != georgia.Mask {
		t.Error("two slots served by one face rasterized it twice")
	}
	if bold := f.Glyph(16, 'a', frame.FontSlot{Family: frame.FontTimes, Bold: true}); bold.Mask == times.Mask {
		t.Error("a bold slot reused the regular face")
	}
}

func TestSlotsNeverCoverLessThanTheEmbeddedFace(t *testing.T) {
	f, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("NewFonts: %v", err)
	}
	for _, r := range []rune{'←', '→', '∑', '√', '✓', '★', '♠', '£', '∞'} {
		for _, slot := range fontSlots {
			if embedded := f.Glyph(16, r, frame.FontSlot{Bold: slot.Bold}); embedded.Ok {
				if got := f.Glyph(16, r, slot); !got.Ok || got.Mask == nil {
					t.Errorf("slot %+v dropped %q that the embedded face has", slot, r)
				}
			}
		}
	}
}

func TestGlyphAtlasKeysSlotsApart(t *testing.T) {
	f, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("NewFonts: %v", err)
	}
	g := raster.NewGlyphAtlas(1<<20, f)
	regular := g.Get('a', 16, frame.FontSlot{Family: frame.FontTimes})
	bold := g.Get('a', 16, frame.FontSlot{Family: frame.FontTimes, Bold: true})
	if regular.Mask == nil || bold.Mask == nil {
		t.Fatal("no glyph mask")
	}
	if regular.Mask == bold.Mask {
		t.Error("the atlas served a bold glyph for a regular request")
	}
	if g.Get('a', 16, frame.FontSlot{Family: frame.FontTimes}).Mask != regular.Mask {
		t.Error("the same slot re-rasterized instead of hitting the cache")
	}
}

func TestGlyphAdvanceFixedIsTheUnroundedAdvance(t *testing.T) {
	f, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("NewFonts: %v", err)
	}
	const size = int32(16)
	for _, slot := range fontSlots {
		fixed := f.GlyphAdvanceFixed(size, 'e', slot)
		if fixed <= 0 {
			t.Fatalf("slot %+v: GlyphAdvanceFixed('e') = %d, want a positive advance", slot, fixed)
		}
		if got, want := f.GlyphAdvance(size, 'e', slot), (fixed+32)>>6; got != want {
			t.Errorf("slot %+v: GlyphAdvance = %d, want %d (fixed %d rounded once)", slot, got, want, fixed)
		}
	}
}

func TestCoverageResolver(t *testing.T) {
	hasHostFallback := false
	for _, p := range []string{
		"/System/Library/Fonts/Apple Symbols.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/truetype/noto/NotoSansSymbols2-Regular.ttf",
		"/usr/share/fonts/truetype/noto/NotoSansSymbols-Regular.ttf",
	} {
		if _, err := os.Stat(p); err == nil {
			hasHostFallback = true
			break
		}
	}
	if !hasHostFallback {
		t.Skip("skipping host font check: no candidate fallback font present on this host")
	}
	f, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("NewFonts: %v", err)
	}
	slot := frame.FontSlot{Family: frame.FontArial}

	faceA, okA := f.FaceForRune(16, 'A', slot)
	if !okA || faceA == nil || !raster.HasGlyph(faceA, 'A') {
		t.Errorf("FaceForRune('A') failed to return a face with HasGlyph == true")
	}

	faceCheck, okCheck := f.FaceForRune(16, '✔', slot)
	if !okCheck || faceCheck == nil {
		t.Fatalf("FaceForRune('✔') = (nil, false), want face from wide-coverage fallback")
	}
	if !raster.HasGlyph(faceCheck, '✔') {
		t.Errorf("FaceForRune('✔') returned face without glyph: HasGlyph == false")
	}

	faceEmoji, okEmoji := f.FaceForRune(16, '😀', slot)
	if okEmoji || faceEmoji != nil {
		t.Errorf("FaceForRune('😀') = (%v, %v), want (nil, false) indicating missing", faceEmoji, okEmoji)
	}
}

func TestCoverageResolverPortable(t *testing.T) {
	f, err := raster.NewEmbeddedFonts()
	if err != nil {
		t.Fatalf("NewEmbeddedFonts: %v", err)
	}
	slot := frame.FontSlot{Family: frame.FontArial}

	faceArrow, okArrow := f.FaceForRune(16, '→', slot)
	if !okArrow || faceArrow == nil || !raster.HasGlyph(faceArrow, '→') {
		t.Errorf("embedded fallback failed to cover '→'")
	}

	faceMissing, okMissing := f.FaceForRune(16, '😀', slot)
	if okMissing || faceMissing != nil {
		t.Errorf("FaceForRune('😀') = (%v, %v), want (nil, false)", faceMissing, okMissing)
	}
}

func TestTofuVisibleAndAdvanceNonZero(t *testing.T) {
	f, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("NewFonts: %v", err)
	}
	const size = int32(16)
	slot := frame.FontSlot{Family: frame.FontTimes}

	g := f.Glyph(size, '😀', slot)
	if !g.Ok {
		t.Fatalf("Glyph('😀').Ok = false, want true for tofu")
	}
	if !g.Missing {
		t.Fatalf("Glyph('😀').Missing = false, want true")
	}
	if g.Mask == nil || g.Mask.Bounds().Empty() {
		t.Fatalf("Glyph('😀').Mask is nil or empty, want visible tofu box")
	}
	if g.Advance <= 0 {
		t.Fatalf("Glyph('😀').Advance = %d, want > 0", g.Advance)
	}

	hasInk := false
	for _, pix := range g.Mask.Pix {
		if pix > 0 {
			hasInk = true
			break
		}
	}
	if !hasInk {
		t.Fatalf("Glyph('😀').Mask has no ink pixels, want bordered box")
	}

	adv := f.GlyphAdvance(size, '😀', slot)
	if adv <= 0 {
		t.Fatalf("GlyphAdvance(size, '😀') = %d, want > 0 (mutation ii invariant)", adv)
	}
	fixed := f.GlyphAdvanceFixed(size, '😀', slot)
	if fixed <= 0 {
		t.Fatalf("GlyphAdvanceFixed(size, '😀') = %d, want > 0 (mutation ii invariant)", fixed)
	}
	if got, want := adv, (fixed+32)>>6; got != want {
		t.Errorf("advance mismatch: GlyphAdvance=%d, fixed rounded=%d", got, want)
	}
}

func TestMissingGlyphCounter(t *testing.T) {
	f, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("NewFonts: %v", err)
	}
	f.ResetMissingCount()
	if got := f.MissingCount(); got != 0 {
		t.Fatalf("MissingCount after reset = %d, want 0", got)
	}

	slot := frame.FontSlot{Family: frame.FontArial}

	for _, r := range "Hello World 123" {
		f.GlyphAdvanceFixed(16, r, slot)
		f.Glyph(16, r, slot)
	}
	if got := f.MissingCount(); got != 0 {
		t.Fatalf("MissingCount after ASCII = %d, want 0", got)
	}

	f.GlyphAdvanceFixed(16, '😀', slot)
	f.Glyph(16, '🎉', slot)
	if got := f.MissingCount(); got <= 0 {
		t.Fatalf("MissingCount after emoji = %d, want > 0 (mutation iii invariant)", got)
	}
}

func TestMacCJKCollectionFallbackDrawsKana(t *testing.T) {
	path := "/System/Library/Fonts/CJKSymbolsFallback.ttc"
	info, err := os.Stat(path)
	if err != nil {
		t.Skip("macOS CJK fallback collection is not installed")
	}
	if info.Size() > raster.MaxFallbackFontBytes {
		t.Fatalf("CJK fallback collection size %d exceeds admission limit %d", info.Size(), raster.MaxFallbackFontBytes)
	}
	f, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("NewFonts: %v", err)
	}
	glyph := f.Glyph(16, 'か', frame.FontSlot{Family: frame.FontArial})
	if glyph.Mask == nil || !glyph.Ok || glyph.Missing {
		t.Fatalf("kana glyph = %+v, want a covered, non-tofu glyph", glyph)
	}
	if f.FallbackFontsCount() == 0 || f.FallbackBytes() > raster.MaxFallbackFontBytes {
		t.Fatalf("fallback count/bytes = %d/%d, want a retained bounded CJK face", f.FallbackFontsCount(), f.FallbackBytes())
	}
}

func TestLazyFallbackHostFonts(t *testing.T) {
	var hookFiles []string
	f, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("NewFonts: %v", err)
	}
	f.SetFallbackHook(func(path string) {
		hookFiles = append(hookFiles, path)
	})

	if got := f.FallbackFilesRead(); got != 0 {
		t.Fatalf("FallbackFilesRead before any miss = %d, want 0", got)
	}
	if len(hookFiles) != 0 {
		t.Fatalf("fallback hook called %d times before any miss, want 0", len(hookFiles))
	}
	if got := f.FallbackFontsCount(); got != 0 {
		t.Fatalf("FallbackFontsCount before any miss = %d, want 0", got)
	}

	slot := frame.FontSlot{Family: frame.FontArial}
	for _, r := range "The quick brown fox jumps over the lazy dog 1234567890" {
		f.GlyphAdvanceFixed(16, r, slot)
		f.Glyph(16, r, slot)
	}
	if got := f.FallbackFilesRead(); got != 0 {
		t.Fatalf("FallbackFilesRead after ASCII text = %d, want 0", got)
	}
	if len(hookFiles) != 0 {
		t.Fatalf("fallback hook called %d times after ASCII, want 0", len(hookFiles))
	}
	if got := f.FallbackFontsCount(); got != 0 {
		t.Fatalf("FallbackFontsCount after ASCII text = %d, want 0", got)
	}

	hasHostFallback := false
	for _, p := range []string{
		"/System/Library/Fonts/Apple Symbols.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/truetype/noto/NotoSansSymbols2-Regular.ttf",
		"/usr/share/fonts/truetype/noto/NotoSansSymbols-Regular.ttf",
	} {
		if _, err := os.Stat(p); err == nil {
			hasHostFallback = true
			break
		}
	}
	if !hasHostFallback {
		t.Skip("skipping host font resolution check: no host fallback font present")
	}

	g := f.Glyph(16, '✔', slot)
	if !g.Ok || g.Missing {
		t.Fatalf("Glyph('✔') failed to resolve from fallback font")
	}

	if got := f.FallbackFilesRead(); got == 0 {
		t.Fatalf("FallbackFilesRead after '✔' = %d, want > 0", got)
	}
	if len(hookFiles) == 0 {
		t.Fatalf("fallback hook not called after '✔', want > 0")
	}
	if got := f.FallbackFontsCount(); got > 1 {
		t.Fatalf("FallbackFontsCount after one rune miss = %d, want <= 1", got)
	}
	if got := f.FallbackFontsCount(); got != 1 {
		t.Fatalf("FallbackFontsCount after '✔' = %d, want exactly 1", got)
	}

	f.ResetMissingCount()
	tofuAdv := f.GlyphAdvance(16, '😀', slot)
	if tofuAdv <= 0 {
		t.Fatalf("tofu GlyphAdvance('😀') = %d, want > 0", tofuAdv)
	}
	if got := f.MissingCount(); got <= 0 {
		t.Fatalf("MissingCount after emoji = %d, want > 0", got)
	}
}

func TestMissingGlyphBatchDoesNotRepeatedlyReadFile(t *testing.T) {
	f, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("NewFonts: %v", err)
	}
	const missingPath = "/nonexistent/font/path/font.ttf"
	f.SetFallbackCandidates([]string{missingPath})

	var hookedPaths []string
	f.SetFallbackHook(func(path string) {
		hookedPaths = append(hookedPaths, path)
	})

	slot := frame.FontSlot{Family: frame.FontArial}
	missingRunes := []rune("😀😁😂😃😄😅😆😉😊😋😎😍😘😗😙😚☺🙂🤗🤩")

	f.Glyph(16, missingRunes[0], slot)
	initialReads := f.FallbackFilesRead()
	initialHooks := len(hookedPaths)

	if initialHooks != 1 {
		t.Fatalf("initialHooks = %d, want 1", initialHooks)
	}
	if initialReads != 0 {
		t.Fatalf("initialReads = %d, want 0", initialReads)
	}

	for _, r := range missingRunes[1:] {
		f.Glyph(16, r, slot)
	}

	afterReads := f.FallbackFilesRead()
	afterHooks := len(hookedPaths)

	if afterReads != initialReads {
		t.Errorf("FallbackFilesRead grew from %d to %d across %d missing runes: candidate files were re-read from disk",
			initialReads, afterReads, len(missingRunes)-1)
	}
	if afterHooks != initialHooks {
		t.Errorf("fallback hook called %d additional times across missing runes (before %d, after %d): candidates were repeatedly probed",
			afterHooks-initialHooks, initialHooks, afterHooks)
	}
}

func TestFallbackAdmissionCeilingRejectsOversizedFont(t *testing.T) {
	arialUnicodePath := "/System/Library/Fonts/Supplemental/Arial Unicode.ttf"
	fi, err := os.Stat(arialUnicodePath)
	if err != nil {
		t.Skip("skipping host font check: Arial Unicode not present on this host")
	}
	if fi.Size() <= raster.MaxFallbackFontBytes {
		t.Fatalf("Arial Unicode size %d is <= MaxFallbackFontBytes %d, test assumption invalid",
			fi.Size(), raster.MaxFallbackFontBytes)
	}

	f, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("NewFonts: %v", err)
	}
	f.SetFallbackCandidates([]string{arialUnicodePath})

	var hooked []string
	f.SetFallbackHook(func(path string) {
		hooked = append(hooked, path)
	})

	f.Glyph(16, '😀', frame.FontSlot{Family: frame.FontArial})

	if len(hooked) != 1 || hooked[0] != arialUnicodePath {
		t.Fatalf("fallback hook called %d times (%v), want 1 call for %s", len(hooked), hooked, arialUnicodePath)
	}
	if got := f.FallbackFilesRead(); got != 0 {
		t.Fatalf("FallbackFilesRead = %d, want 0 (oversized font must be rejected before ReadFile)", got)
	}
	if got := f.FallbackFontsCount(); got != 0 {
		t.Fatalf("FallbackFontsCount = %d, want 0", got)
	}
	if got := f.FallbackBytes(); got != 0 {
		t.Fatalf("FallbackBytes = %d, want 0", got)
	}

	f.Glyph(16, '😁', frame.FontSlot{Family: frame.FontArial})
	if len(hooked) != 1 {
		t.Fatalf("fallback hook called %d times, want 1 (memoized rejection)", len(hooked))
	}
	if got := f.FallbackFilesRead(); got != 0 {
		t.Fatalf("FallbackFilesRead = %d after second rune miss, want 0", got)
	}
}

func TestMissingCandidateWithCoverageDoesNotRetainFace(t *testing.T) {
	hasHostFallback := false
	for _, p := range []string{
		"/System/Library/Fonts/Apple Symbols.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	} {
		if _, err := os.Stat(p); err == nil {
			hasHostFallback = true
			break
		}
	}
	if !hasHostFallback {
		t.Skip("skipping host font check: no candidate fallback font present on this host")
	}

	f, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("NewFonts: %v", err)
	}
	slot := frame.FontSlot{Family: frame.FontArial}

	f.Glyph(16, '😀', slot)

	if got := f.FallbackFontsCount(); got != 0 {
		t.Fatalf("FallbackFontsCount after missing emoji = %d, want 0 (unhelpful faces must not be retained)", got)
	}
	if got := f.FallbackBytes(); got != 0 {
		t.Fatalf("FallbackBytes after missing emoji = %d, want 0", got)
	}
	readsAfterFirstMiss := f.FallbackFilesRead()
	if readsAfterFirstMiss == 0 {
		t.Fatalf("FallbackFilesRead after first miss = 0, want > 0")
	}

	missingRunes := []rune("😁😂😃😄😅😆😉😊😋")
	for _, r := range missingRunes {
		f.Glyph(16, r, slot)
	}
	if got := f.FallbackFilesRead(); got != readsAfterFirstMiss {
		t.Fatalf("FallbackFilesRead grew from %d to %d across %d missing emojis",
			readsAfterFirstMiss, got, len(missingRunes))
	}
	if got := f.FallbackFontsCount(); got != 0 {
		t.Fatalf("FallbackFontsCount = %d, want 0", got)
	}

	gCheck := f.Glyph(16, '✔', slot)
	if !gCheck.Ok || gCheck.Missing {
		t.Fatalf("Glyph('✔') failed to resolve")
	}
	if got := f.FallbackFontsCount(); got != 1 {
		t.Fatalf("FallbackFontsCount after '✔' = %d, want exactly 1 (useful face admitted)", got)
	}
	if got := f.FallbackBytes(); got <= 0 {
		t.Fatalf("FallbackBytes after '✔' = %d, want > 0", got)
	}
}

func TestPositiveGlyphOracleByScriptGroup(t *testing.T) {
	hasHostFallback := false
	for _, p := range []string{
		"/System/Library/Fonts/Apple Symbols.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/dejavu/DejaVuSans.ttf",
	} {
		if _, err := os.Stat(p); err == nil {
			hasHostFallback = true
			break
		}
	}
	if !hasHostFallback {
		t.Skip("skipping host font check: no candidate fallback font present on this host")
	}

	f, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("NewFonts: %v", err)
	}
	slot := frame.FontSlot{Family: frame.FontArial}

	groups := []struct {
		name     string
		lo, hi   rune
		gapRunes []rune
	}{
		{"Greek", 0x0391, 0x03C9, []rune{0x03A2}},
		{"Cyrillic", 0x0410, 0x0441, nil},
		{"Hebrew", 0x05D0, 0x05EA, nil},
		{"Arabic", 0x0621, 0x0648, []rune{0x063B, 0x063C, 0x063D, 0x063E, 0x063F}},
		{"Box Drawing", 0x2500, 0x2527, nil},
		{"Arrows", 0x2190, 0x21B7, nil},
		{"Math Operators", 0x2200, 0x2231, nil},
		{"Symbols & Dingbats", 0x2600, 0x263B, nil},
	}

	for _, grp := range groups {
		t.Run(grp.name, func(t *testing.T) {
			f.ResetMissingCount()
			declared := map[rune]bool{}
			for _, r := range grp.gapRunes {
				declared[r] = true
			}
			var actual []rune
			for r := grp.lo; r <= grp.hi; r++ {
				g := f.Glyph(16, r, slot)
				switch {
				case g.Missing:
					actual = append(actual, r)
					if !declared[r] {
						t.Errorf("script %s: rune U+%04X misses but is not a declared gap", grp.name, r)
					}
				case !g.Ok:
					t.Errorf("script %s: rune U+%04X neither resolved nor marked missing (Ok=%v Missing=%v)",
						grp.name, r, g.Ok, g.Missing)
				default:
					if declared[r] {
						t.Errorf("script %s: declared gap rune U+%04X now resolves (Ok=%v Missing=%v): the gap list is stale",
							grp.name, r, g.Ok, g.Missing)
					}
				}
			}
			if len(actual) != len(grp.gapRunes) {
				t.Errorf("script %s: %d runes miss, want %d declared gaps %v (actual %v)",
					grp.name, len(actual), len(grp.gapRunes), grp.gapRunes, actual)
			}
			if got := f.MissingCount(); got != int64(len(actual)) {
				t.Errorf("script %s: MissingCount = %d, want %d (one increment per missing Glyph lookup)",
					grp.name, got, len(actual))
			}

			f.ResetMissingCount()
			for r := grp.lo; r <= grp.hi; r++ {
				if adv := f.GlyphAdvanceFixed(16, r, slot); adv <= 0 {
					t.Fatalf("script %s: rune U+%04X non-positive advance: %d", grp.name, r, adv)
				}
			}
			if got := f.MissingCount(); got != int64(len(grp.gapRunes)) {
				t.Errorf("script %s: MissingCount after the advance pass = %d, want %d",
					grp.name, got, len(grp.gapRunes))
			}
		})
	}

	t.Run("CJK_KnownGap", func(t *testing.T) {
		f.ResetMissingCount()
		cjkRunes := []rune{'中', '文'}
		for _, r := range cjkRunes {
			g := f.Glyph(16, r, slot)
			if !g.Ok || !g.Missing {
				t.Fatalf("CJK rune %c (U+%04X) expected tofu fallback (Ok=true, Missing=true), got Ok=%v, Missing=%v",
					r, r, g.Ok, g.Missing)
			}
			if g.Advance <= 0 {
				t.Fatalf("CJK rune %c tofu advance %d <= 0", r, g.Advance)
			}
		}
		if got := f.MissingCount(); got != int64(len(cjkRunes)) {
			t.Errorf("MissingCount after %d CJK runes = %d, want %d", len(cjkRunes), got, len(cjkRunes))
		}
	})
}

func TestMeasureHotPathLatency(t *testing.T) {
	f, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("NewFonts: %v", err)
	}
	slot := frame.FontSlot{Family: frame.FontArial}
	f.Glyph(16, 'A', slot)

	startFirst := time.Now()
	f.Glyph(16, '中', slot)
	dFirst := time.Since(startFirst)

	missingRunes := []rune("文語漢字日本語한국어😀😁😂😃😄")
	startSub := time.Now()
	for _, r := range missingRunes {
		f.Glyph(16, r, slot)
	}
	dSub := time.Since(startSub)
	dSubAvg := dSub / time.Duration(len(missingRunes))

	t.Logf("Hot path latency: first miss = %v (%.3f ms), subsequent avg = %v (%.4f ms), total %d runes = %v",
		dFirst, float64(dFirst.Nanoseconds())/1e6,
		dSubAvg, float64(dSubAvg.Nanoseconds())/1e6,
		len(missingRunes), dSub)
}
