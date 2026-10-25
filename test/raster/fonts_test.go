package raster

import (
	"bytes"
	"os"
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/raster"
)

// fontSlots is one representative of every face a resolved style can ask for.
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

// TestFontsWithoutSystemFontsStillDraw is the determinism contract: a host with
// no system fonts - which is what a Linux CI runner is - must still construct a
// font set and serve every slot, because a slot falls back to the embedded face
// rather than failing the page.
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

// TestSystemFontFlagFallsBackPerSlot checks the fallback is per slot rather than
// per run: with the host fonts off, every family draws the embedded face at the
// slot's own weight.
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

// TestSlotsSharingAFaceShareItsGlyphs: a fallback is a face, not a copy. With
// the host fonts off every family is served by the embedded one, and a glyph
// rasterized for Times is the same bitmap Georgia asks for - which is also what
// keeps measurement and paint from ever disagreeing.
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

// TestSlotsNeverCoverLessThanTheEmbeddedFace: a text family has no arrow, no
// summation sign and no check mark, so those fall back to the embedded face
// rather than disappearing from the page. This holds on a host with the fonts
// and on one without them, which is why it compares against the embedded slot
// instead of a fixed answer.
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

// TestGlyphAtlasKeysSlotsApart: the atlas caches per requested slot, so a page
// that draws one glyph in two weights never reads back the other's mask.
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

// TestGlyphAdvanceFixedIsTheUnroundedAdvance pins the fractional-measurement
// contract: GlyphAdvanceFixed carries the face's raw 26.6 advance, and the
// integer GlyphAdvance is exactly that value rounded once. A run measured by
// summing the fixed values must disagree with summing the rounded ones when
// the advance has a fraction, which is the whole reason the fixed API exists.
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

// TestCoverageResolver verifies AC-04:
// For a rune outside mapped slots ('✔'), FaceForRune must return a face where HasGlyph(face, '✔') == true.
// For a rune not covered by any face ('😀'), it must return (nil, false) as the missing signal.
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

	// 1. ASCII rune should be served by the requested face.
	faceA, okA := f.FaceForRune(16, 'A', slot)
	if !okA || faceA == nil || !raster.HasGlyph(faceA, 'A') {
		t.Errorf("FaceForRune('A') failed to return a face with HasGlyph == true")
	}

	// 2. Rune '✔' (U+2714) outside standard Arial/Times slots must resolve to a fallback face (e.g. Arial Unicode)
	// that actually has the glyph.
	faceCheck, okCheck := f.FaceForRune(16, '✔', slot)
	if !okCheck || faceCheck == nil {
		t.Fatalf("FaceForRune('✔') = (nil, false), want face from wide-coverage fallback")
	}
	if !raster.HasGlyph(faceCheck, '✔') {
		t.Errorf("FaceForRune('✔') returned face without glyph: HasGlyph == false")
	}

	// 3. Emoji rune '😀' (U+1F600) covered by no face must return (nil, false) as missing signal,
	// rather than a face that cannot draw it.
	faceEmoji, okEmoji := f.FaceForRune(16, '😀', slot)
	if okEmoji || faceEmoji != nil {
		t.Errorf("FaceForRune('😀') = (%v, %v), want (nil, false) indicating missing", faceEmoji, okEmoji)
	}
}

// TestCoverageResolverPortable ensures that even on a host without system fonts (like Linux CI),
// wide-coverage fallback chaining via AddFallback functions correctly.
func TestCoverageResolverPortable(t *testing.T) {
	f, err := raster.NewEmbeddedFonts()
	if err != nil {
		t.Fatalf("NewEmbeddedFonts: %v", err)
	}
	slot := frame.FontSlot{Family: frame.FontArial}

	// Embedded face has arrows ('→').
	faceArrow, okArrow := f.FaceForRune(16, '→', slot)
	if !okArrow || faceArrow == nil || !raster.HasGlyph(faceArrow, '→') {
		t.Errorf("embedded fallback failed to cover '→'")
	}

	// Emoji has no coverage across embedded faces -> must report missing.
	faceMissing, okMissing := f.FaceForRune(16, '😀', slot)
	if okMissing || faceMissing != nil {
		t.Errorf("FaceForRune('😀') = (%v, %v), want (nil, false)", faceMissing, okMissing)
	}
}

// TestTofuVisibleAndAdvanceNonZero tests AC-04 and requirement (ii):
// A missing glyph must produce visible tofu (bordered rectangle mask) with stable non-zero advance.
func TestTofuVisibleAndAdvanceNonZero(t *testing.T) {
	f, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("NewFonts: %v", err)
	}
	const size = int32(16)
	slot := frame.FontSlot{Family: frame.FontTimes}

	// Glyph check
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

	// Mask must have ink (non-zero alpha)
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

	// Advance checks
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

// TestMissingGlyphCounter tests requirement (iii):
// A controlled counter tracks missing glyphs that fall back to tofu.
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

	// Measuring / rasterizing ASCII must produce ZERO missing glyphs.
	for _, r := range "Hello World 123" {
		f.GlyphAdvanceFixed(16, r, slot)
		f.Glyph(16, r, slot)
	}
	if got := f.MissingCount(); got != 0 {
		t.Fatalf("MissingCount after ASCII = %d, want 0", got)
	}

	// Measuring / rasterizing missing runes (e.g. emoji) must increment counter.
	f.GlyphAdvanceFixed(16, '😀', slot)
	f.Glyph(16, '🎉', slot)
	if got := f.MissingCount(); got <= 0 {
		t.Fatalf("MissingCount after emoji = %d, want > 0 (mutation iii invariant)", got)
	}
}

// TestLazyFallbackHostFonts tests requirement 4 and [R-7]:
// 1. Files read via hook/counter is 0 before any rune miss.
// 2. After one rune requiring fallback (e.g. '✔'), at most 1 font is retained.
// 3. MissingCount and tofu advance remain correct as in §6.
func TestLazyFallbackHostFonts(t *testing.T) {
	var hookFiles []string
	f, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("NewFonts: %v", err)
	}
	f.SetFallbackHook(func(path string) {
		hookFiles = append(hookFiles, path)
	})

	// Before any rune miss, host fallback files read must be 0 and no fallback fonts retained.
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

	// Trigger fallback with a rune outside standard slots ('✔')
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

	// After one rune needing fallback:
	// - Files read > 0 (verified hook called)
	// - Number of retained fallback fonts MUST BE <= 1
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

	// Verify MissingCount and tofu advance remain intact
	f.ResetMissingCount()
	tofuAdv := f.GlyphAdvance(16, '😀', slot)
	if tofuAdv <= 0 {
		t.Fatalf("tofu GlyphAdvance('😀') = %d, want > 0", tofuAdv)
	}
	if got := f.MissingCount(); got <= 0 {
		t.Fatalf("MissingCount after emoji = %d, want > 0", got)
	}
}

// TestMissingGlyphBatchDoesNotRepeatedlyReadFile satisfies requirement for Issue 3:
// When rasterizing a batch of missing glyphs (none covered by any fallback),
// candidate files must NOT be repeatedly stat-ed or read on each missing glyph.
// Both file reads and candidate probes must remain strictly bounded across all runes.
func TestMissingGlyphBatchDoesNotRepeatedlyReadFile(t *testing.T) {
	f, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("NewFonts: %v", err)
	}
	var hookedPaths []string
	f.SetFallbackHook(func(path string) {
		hookedPaths = append(hookedPaths, path)
	})

	slot := frame.FontSlot{Family: frame.FontArial}
	// A series of distinct missing emoji runes
	missingRunes := []rune("😀😁😂😃😄😅😆😉😊😋😎😍😘😗😙😚☺🙂🤗🤩")

	// Rasterize the first missing rune - this triggers the one-time probe of candidate paths.
	f.Glyph(16, missingRunes[0], slot)
	initialReads := f.FallbackFilesRead()
	initialHooks := len(hookedPaths)

	// Rasterize all subsequent missing runes - all candidates are already memoized.
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

// TestFallbackAdmissionCeilingRejectsOversizedFont satisfies requirement for Issue 2:
// Files exceeding raster.MaxFallbackFontBytes (such as Arial Unicode.ttf at 22-39 MiB)
// must be rejected at admission and never read into memory.
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
	// Query a missing rune; Arial Unicode must be rejected due to size > MaxFallbackFontBytes.
	f.Glyph(16, '😀', frame.FontSlot{Family: frame.FontArial})

	// Retained bytes must be strictly within admission budget and well below Arial Unicode.
	if retained := f.FallbackBytes(); retained >= fi.Size() {
		t.Errorf("f.FallbackBytes() = %d retained oversized font (%d bytes)", retained, fi.Size())
	}
	if f.FallbackBytes() > int64(raster.MaxParsedFallbackFonts*raster.MaxFallbackFontBytes) {
		t.Errorf("f.FallbackBytes() = %d exceeds configured fallback budget", f.FallbackBytes())
	}
}
