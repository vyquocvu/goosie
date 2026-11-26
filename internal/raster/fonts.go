package raster

import (
	"encoding/binary"
	"image"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/vyquocvu/goosie/internal/frame"
)

const faceDPI = 72

type Glyph struct {
	Mask    *image.Alpha
	Bounds  frame.Rect
	Advance int32
	Ok      bool
	Missing bool
}

func (g Glyph) Bytes() int64 {
	if g.Mask == nil {
		return 0
	}
	return int64(len(g.Mask.Pix))
}

type faceKey struct {
	slot frame.FontSlot
	size int32
}

type fallbackKey struct {
	idx  int
	size int32
}

type runeRange struct {
	start rune
	end   rune
}

type runeCoverage []runeRange

type fallbackCandidate struct {
	path       string
	runeRanges runeCoverage
}

func (rc runeCoverage) contains(r rune) bool {
	i := sort.Search(len(rc), func(i int) bool {
		return rc[i].end >= r
	})
	if i < len(rc) && rc[i].start <= r {
		return true
	}
	return false
}

func parseCmapRanges(data []byte) runeCoverage {
	if len(data) < 12 {
		return nil
	}
	offsets := []int{0}
	if string(data[:4]) == "ttcf" {
		if len(data) < 12 {
			return nil
		}
		count := binary.BigEndian.Uint32(data[8:12])
		if count == 0 || count > 64 || uint64(12)+uint64(count)*4 > uint64(len(data)) {
			return nil
		}
		offsets = make([]int, 0, count)
		for i := uint32(0); i < count; i++ {
			offset := binary.BigEndian.Uint32(data[12+i*4 : 16+i*4])
			if uint64(offset)+12 <= uint64(len(data)) {
				offsets = append(offsets, int(offset))
			}
		}
	}
	var ranges []runeRange
	for _, offset := range offsets {
		ranges = append(ranges, parseSFNTCmapRangesAt(data, offset)...)
	}
	if len(ranges) == 0 {
		return nil
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].start < ranges[j].start })
	merged := make([]runeRange, 0, len(ranges))
	curr := ranges[0]
	for _, next := range ranges[1:] {
		if next.start <= curr.end+1 {
			if next.end > curr.end {
				curr.end = next.end
			}
		} else {
			merged = append(merged, curr)
			curr = next
		}
	}
	merged = append(merged, curr)
	return runeCoverage(merged)
}

func parseSFNTCmapRangesAt(data []byte, offset int) []runeRange {
	if offset < 0 || offset+12 > len(data) {
		return nil
	}
	numTables := binary.BigEndian.Uint16(data[offset+4 : offset+6])
	if offset+12+int(numTables)*16 > len(data) {
		return nil
	}
	var cmapOffset, cmapLen uint32
	for i := 0; i < int(numTables); i++ {
		entry := data[offset+12+i*16 : offset+12+(i+1)*16]
		if string(entry[:4]) == "cmap" {
			cmapOffset = binary.BigEndian.Uint32(entry[8:12])
			cmapLen = binary.BigEndian.Uint32(entry[12:16])
			break
		}
	}
	if cmapOffset == 0 || int(cmapOffset+cmapLen) > len(data) {
		return nil
	}

	cmap := data[cmapOffset : cmapOffset+cmapLen]
	if len(cmap) < 4 {
		return nil
	}
	subTables := binary.BigEndian.Uint16(cmap[2:4])
	if len(cmap) < int(4+subTables*8) {
		return nil
	}

	var fmt12Offset, fmt4Offset uint32
	for i := 0; i < int(subTables); i++ {
		rec := cmap[4+i*8 : 4+(i+1)*8]
		pid := binary.BigEndian.Uint16(rec[:2])
		psid := binary.BigEndian.Uint16(rec[2:4])
		offset := binary.BigEndian.Uint32(rec[4:8])
		if int(offset+2) > len(cmap) {
			continue
		}
		format := binary.BigEndian.Uint16(cmap[offset : offset+2])
		if format == 12 && (pid == 0 || (pid == 3 && psid == 10)) {
			fmt12Offset = offset
		} else if format == 4 && (pid == 0 || (pid == 3 && psid == 1)) && fmt4Offset == 0 {
			fmt4Offset = offset
		}
	}

	var ranges []runeRange
	if fmt12Offset > 0 && int(fmt12Offset+16) <= len(cmap) {
		sub := cmap[fmt12Offset:]
		numGroups := binary.BigEndian.Uint32(sub[12:16])
		if int(16+numGroups*12) <= len(sub) {
			for i := uint32(0); i < numGroups; i++ {
				g := sub[16+i*12 : 16+(i+1)*12]
				start := binary.BigEndian.Uint32(g[:4])
				end := binary.BigEndian.Uint32(g[4:8])
				ranges = append(ranges, runeRange{start: rune(start), end: rune(end)})
			}
		}
	} else if fmt4Offset > 0 && int(fmt4Offset+14) <= len(cmap) {
		sub := cmap[fmt4Offset:]
		segCountX2 := binary.BigEndian.Uint16(sub[6:8])
		segCount := int(segCountX2 / 2)
		endCodesOffset := 14
		startCodesOffset := 14 + segCount*2 + 2
		if startCodesOffset+segCount*2 <= len(sub) {
			for i := 0; i < segCount; i++ {
				end := binary.BigEndian.Uint16(sub[endCodesOffset+i*2 : endCodesOffset+(i+1)*2])
				start := binary.BigEndian.Uint16(sub[startCodesOffset+i*2 : startCodesOffset+(i+1)*2])
				if start == 0xFFFF {
					break
				}
				ranges = append(ranges, runeRange{start: rune(start), end: rune(end)})
			}
		}
	}

	if len(ranges) == 0 {
		return nil
	}
	return ranges
}

type Fonts struct {
	mu                    sync.RWMutex
	sources               map[frame.FontSlot]*opentype.Font
	entries               map[faceKey]*faceEntry
	fbCache               map[faceKey][]*faceEntry
	custom                []*opentype.Font
	fallbacks             []*opentype.Font
	fbEntries             map[fallbackKey]*faceEntry
	candidateCoverage     map[string]runeCoverage
	rejectedFallbackPaths map[string]bool
	retainedFallbackPaths map[string]bool
	fallbackCandidates    []fallbackCandidate
	fallbackBytes         int64
	fallbackFilesRead     atomic.Int64
	fallbackHook          func(path string)
	missingCount          atomic.Int64
}

type faceEntry struct {
	face    font.Face
	glyphs  map[rune]Glyph
	missing map[rune]struct{}
	adv     map[rune]int32
	advMiss map[rune]struct{}
}

const MaxFallbackFontBytes = 4 * 1024 * 1024

const MaxParsedFallbackFonts = 2

const systemFontDir = "/System/Library/Fonts/Supplemental"

const hiraginoSansW0Path = "/System/Library/Fonts/ヒラギノ角ゴシック W0.ttc"

var systemFontFiles = []struct {
	slot frame.FontSlot
	file string
}{
	{frame.FontSlot{Family: frame.FontTimes}, "Times New Roman.ttf"},
	{frame.FontSlot{Family: frame.FontTimes, Bold: true}, "Times New Roman Bold.ttf"},
	{frame.FontSlot{Family: frame.FontTimes, Italic: true}, "Times New Roman Italic.ttf"},
	{frame.FontSlot{Family: frame.FontTimes, Bold: true, Italic: true}, "Times New Roman Bold Italic.ttf"},
	{frame.FontSlot{Family: frame.FontArial}, "Arial.ttf"},
	{frame.FontSlot{Family: frame.FontArial, Bold: true}, "Arial Bold.ttf"},
	{frame.FontSlot{Family: frame.FontArial, Italic: true}, "Arial Italic.ttf"},
	{frame.FontSlot{Family: frame.FontArial, Bold: true, Italic: true}, "Arial Bold Italic.ttf"},
	{frame.FontSlot{Family: frame.FontCourier}, "Courier New.ttf"},
	{frame.FontSlot{Family: frame.FontCourier, Bold: true}, "Courier New Bold.ttf"},
	{frame.FontSlot{Family: frame.FontCourier, Italic: true}, "Courier New Italic.ttf"},
	{frame.FontSlot{Family: frame.FontCourier, Bold: true, Italic: true}, "Courier New Bold Italic.ttf"},
	{frame.FontSlot{Family: frame.FontGeorgia}, "Georgia.ttf"},
	{frame.FontSlot{Family: frame.FontGeorgia, Bold: true}, "Georgia Bold.ttf"},
	{frame.FontSlot{Family: frame.FontGeorgia, Italic: true}, "Georgia Italic.ttf"},
	{frame.FontSlot{Family: frame.FontGeorgia, Bold: true, Italic: true}, "Georgia Bold Italic.ttf"},
	{frame.FontSlot{Family: frame.FontVerdana}, "Verdana.ttf"},
	{frame.FontSlot{Family: frame.FontVerdana, Bold: true}, "Verdana Bold.ttf"},
	{frame.FontSlot{Family: frame.FontVerdana, Italic: true}, "Verdana Italic.ttf"},
	{frame.FontSlot{Family: frame.FontVerdana, Bold: true, Italic: true}, "Verdana Bold Italic.ttf"},
}

var systemFontCollections = []struct {
	slot frame.FontSlot
	path string
	idx  int
}{
	{frame.FontSlot{Family: frame.FontHelvetica}, "/System/Library/Fonts/HelveticaNeue.ttc", 2},
	{frame.FontSlot{Family: frame.FontHelvetica, Bold: true}, "/System/Library/Fonts/HelveticaNeue.ttc", 10},
	{frame.FontSlot{Family: frame.FontHelvetica, Light: true}, "/System/Library/Fonts/HelveticaNeue.ttc", 12},
}

var systemFallbackCandidates = []fallbackCandidate{
	{path: "/System/Library/Fonts/CJKSymbolsFallback.ttc"},
	{
		path: hiraginoSansW0Path,
		runeRanges: runeCoverage{
			{start: 0x2E80, end: 0x2FDF},
			{start: 0x3007, end: 0x3007},
			{start: 0x3400, end: 0x4DBF},
			{start: 0x4E00, end: 0x9FFF},
			{start: 0xF900, end: 0xFAFF},
			{start: 0x20000, end: 0x2A6DF},
			{start: 0x2A700, end: 0x2B73F},
			{start: 0x2B740, end: 0x2B81F},
			{start: 0x2B820, end: 0x2CEAF},
			{start: 0x2CEB0, end: 0x2EBEF},
			{start: 0x2EBF0, end: 0x2EE5F},
			{start: 0x2F800, end: 0x2FA1F},
			{start: 0x30000, end: 0x3134F},
			{start: 0x31350, end: 0x323AF},
			{start: 0x323B0, end: 0x33479},
		},
	},
	{path: "/System/Library/Fonts/Apple Symbols.ttf"},
	{path: "/System/Library/Fonts/ZapfDingbats.ttf"},
	{path: "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"},
	{path: "/usr/share/fonts/dejavu/DejaVuSans.ttf"},
	{path: "/usr/share/fonts/truetype/noto/NotoSansSymbols2-Regular.ttf"},
	{path: "/usr/share/fonts/truetype/noto/NotoSansSymbols-Regular.ttf"},
	{path: "/usr/share/fonts/noto/NotoSansSymbols2-Regular.ttf"},
	{path: "/usr/share/fonts/noto/NotoSansSymbols-Regular.ttf"},
	{path: "/usr/share/fonts/opentype/noto/NotoSansSymbols2-Regular.otf"},
	{path: "/usr/share/fonts/truetype/freefont/FreeSans.ttf"},
	{path: "/System/Library/Fonts/Supplemental/Arial Unicode.ttf"},
	{path: "/Library/Fonts/Arial Unicode.ttf"},
}

func NewFonts() (*Fonts, error) {
	f, err := NewEmbeddedFonts()
	if err != nil {
		return nil, err
	}
	if systemFontsDisabled() {
		return f, nil
	}
	for _, sf := range systemFontFiles {
		data, err := os.ReadFile(systemFontDir + "/" + sf.file)
		if err != nil {
			continue
		}
		parsed, err := opentype.Parse(data)
		if err != nil {
			continue
		}
		f.sources[sf.slot] = parsed
	}
	for _, sc := range systemFontCollections {
		data, err := os.ReadFile(sc.path)
		if err != nil {
			continue
		}
		col, err := opentype.ParseCollection(data)
		if err != nil || sc.idx >= col.NumFonts() {
			continue
		}
		parsed, err := col.Font(sc.idx)
		if err != nil {
			continue
		}
		f.sources[sc.slot] = parsed
	}
	return f, nil
}

func NewEmbeddedFonts() (*Fonts, error) {
	regular, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, err
	}
	bold, err := opentype.Parse(gobold.TTF)
	if err != nil {
		return nil, err
	}
	return &Fonts{
		sources: map[frame.FontSlot]*opentype.Font{
			{}:           regular,
			{Bold: true}: bold,
		},
		entries:               map[faceKey]*faceEntry{},
		fbCache:               map[faceKey][]*faceEntry{},
		fbEntries:             map[fallbackKey]*faceEntry{},
		candidateCoverage:     map[string]runeCoverage{},
		rejectedFallbackPaths: map[string]bool{},
		retainedFallbackPaths: map[string]bool{},
	}, nil
}

func (f *Fonts) AddFallback(font *opentype.Font) {
	if font == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fallbacks = append(f.fallbacks, font)
	f.fbCache = map[faceKey][]*faceEntry{}
}

func (f *Fonts) fallbackFaceLocked(idx int, size int32, src *opentype.Font) *faceEntry {
	if size <= 0 || src == nil {
		return nil
	}
	k := fallbackKey{idx: idx, size: size}
	if e, ok := f.fbEntries[k]; ok {
		return e
	}
	face, err := opentype.NewFace(src, &opentype.FaceOptions{
		Size:    float64(size),
		DPI:     faceDPI,
		Hinting: font.HintingNone,
	})
	if err != nil {
		return nil
	}
	e := &faceEntry{
		face:    face,
		glyphs:  map[rune]Glyph{},
		missing: map[rune]struct{}{},
		adv:     map[rune]int32{},
		advMiss: map[rune]struct{}{},
	}
	if f.fbEntries == nil {
		f.fbEntries = map[fallbackKey]*faceEntry{}
	}
	f.fbEntries[k] = e
	return e
}

func (f *Fonts) admitAndRetainFallbackLocked(path string, data []byte, size int32, r rune) *faceEntry {
	if len(f.fallbacks) >= MaxParsedFallbackFonts {
		return nil
	}
	var parsed *opentype.Font
	var face font.Face
	if len(data) >= 4 && string(data[:4]) == "ttcf" {
		collection, err := opentype.ParseCollection(data)
		if err != nil {
			f.rejectedFallbackPaths[path] = true
			return nil
		}
		for i := 0; i < collection.NumFonts(); i++ {
			candidate, err := collection.Font(i)
			if err != nil {
				continue
			}
			candidateFace, err := opentype.NewFace(candidate, &opentype.FaceOptions{
				Size: float64(size), DPI: faceDPI, Hinting: font.HintingNone,
			})
			if err == nil && HasGlyph(candidateFace, r) {
				parsed, face = candidate, candidateFace
				break
			}
		}
	} else {
		candidate, err := opentype.Parse(data)
		if err != nil {
			f.rejectedFallbackPaths[path] = true
			return nil
		}
		candidateFace, err := opentype.NewFace(candidate, &opentype.FaceOptions{
			Size: float64(size), DPI: faceDPI, Hinting: font.HintingNone,
		})
		if err == nil && HasGlyph(candidateFace, r) {
			parsed, face = candidate, candidateFace
		}
	}
	if parsed == nil || face == nil {
		f.rejectedFallbackPaths[path] = true
		return nil
	}

	idx := len(f.fallbacks)
	f.fallbacks = append(f.fallbacks, parsed)
	f.retainedFallbackPaths[path] = true
	f.fallbackBytes += int64(len(data))

	e := &faceEntry{
		face:    face,
		glyphs:  map[rune]Glyph{},
		missing: map[rune]struct{}{},
		adv:     map[rune]int32{},
		advMiss: map[rune]struct{}{},
	}
	if f.fbEntries == nil {
		f.fbEntries = map[fallbackKey]*faceEntry{}
	}
	f.fbEntries[fallbackKey{idx: idx, size: size}] = e
	f.fbCache = map[faceKey][]*faceEntry{}
	return e
}

func (f *Fonts) lazyFallbackForRuneLocked(r rune, size int32) *faceEntry {
	if size <= 0 || systemFontsDisabled() {
		return nil
	}
	if isDefaultIgnorable(r) {
		return nil
	}
	if f.candidateCoverage == nil {
		f.candidateCoverage = make(map[string]runeCoverage)
	}
	if f.rejectedFallbackPaths == nil {
		f.rejectedFallbackPaths = make(map[string]bool)
	}
	if f.retainedFallbackPaths == nil {
		f.retainedFallbackPaths = make(map[string]bool)
	}

	candidates := f.fallbackCandidates
	if len(candidates) == 0 {
		candidates = systemFallbackCandidates
	}

	for _, candidate := range candidates {
		path := candidate.path
		if len(candidate.runeRanges) > 0 && !candidate.runeRanges.contains(r) {
			continue
		}
		if f.retainedFallbackPaths[path] {
			continue
		}
		if f.rejectedFallbackPaths[path] {
			continue
		}

		if cov, ok := f.candidateCoverage[path]; ok {
			if !cov.contains(r) {
				continue
			}
			if len(f.fallbacks) >= MaxParsedFallbackFonts {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				f.rejectedFallbackPaths[path] = true
				continue
			}
			if e := f.admitAndRetainFallbackLocked(path, data, size, r); e != nil {
				return e
			}
			continue
		}

		if f.fallbackHook != nil {
			f.fallbackHook(path)
		}

		st, err := os.Stat(path)
		if err != nil {
			f.rejectedFallbackPaths[path] = true
			continue
		}
		if st.Size() <= 0 || st.Size() > MaxFallbackFontBytes {
			f.rejectedFallbackPaths[path] = true
			continue
		}

		f.fallbackFilesRead.Add(1)

		data, err := os.ReadFile(path)
		if err != nil {
			f.rejectedFallbackPaths[path] = true
			continue
		}

		cov := parseCmapRanges(data)
		f.candidateCoverage[path] = cov

		if cov != nil && !cov.contains(r) {
			continue
		}

		if e := f.admitAndRetainFallbackLocked(path, data, size, r); e != nil {
			return e
		}
	}
	return nil
}

func (f *Fonts) FallbackFilesRead() int64 {
	return f.fallbackFilesRead.Load()
}

func (f *Fonts) FallbackFontsCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.fallbacks)
}

func (f *Fonts) FallbackBytes() int64 {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.fallbackBytes
}

func (f *Fonts) SetFallbackCandidates(paths []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(paths) == 0 {
		f.fallbackCandidates = nil
		return
	}
	f.fallbackCandidates = make([]fallbackCandidate, len(paths))
	for i, path := range paths {
		f.fallbackCandidates[i].path = path
	}
}

func (f *Fonts) SetFallbackHook(fn func(path string)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fallbackHook = fn
}

func (f *Fonts) Register(data []byte) (uint16, error) {
	parsed, err := opentype.Parse(data)
	if err != nil {
		return 0, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.custom = append(f.custom, parsed)
	return uint16(len(f.custom)), nil
}

func systemFontsDisabled() bool {
	switch strings.ToLower(os.Getenv("GOOSIE_SYSTEM_FONTS")) {
	case "0", "off", "no", "false":
		return true
	}
	return false
}

func (f *Fonts) Face(size int32, slot frame.FontSlot) font.Face {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e := f.faceLocked(size, slot); e != nil {
		return e.face
	}
	return nil
}

func (f *Fonts) faceLocked(size int32, slot frame.FontSlot) *faceEntry {
	if size <= 0 {
		return nil
	}
	slot, src := f.resolveLocked(slot)
	if src == nil {
		return nil
	}
	k := faceKey{slot: slot, size: size}
	if e, ok := f.entries[k]; ok {
		return e
	}
	face, err := opentype.NewFace(src, &opentype.FaceOptions{
		Size:    float64(size),
		DPI:     faceDPI,
		Hinting: font.HintingNone,
	})
	if err != nil {
		return nil
	}
	e := &faceEntry{face: face, glyphs: map[rune]Glyph{}, missing: map[rune]struct{}{}, adv: map[rune]int32{}, advMiss: map[rune]struct{}{}}
	f.entries[k] = e
	f.fbCache = map[faceKey][]*faceEntry{}
	return e
}

func (f *Fonts) resolveLocked(slot frame.FontSlot) (frame.FontSlot, *opentype.Font) {
	var src *opentype.Font
	if slot.CustomIdx != 0 && int(slot.CustomIdx-1) < len(f.custom) {
		src = f.custom[slot.CustomIdx-1]
	} else {
		src = f.sources[slot]
		if src == nil && slot.Italic {
			slot.Italic = false
			src = f.sources[slot]
		}
		if src == nil && slot.Light {
			slot.Light = false
			src = f.sources[slot]
		}
		if src == nil {
			slot = frame.FontSlot{Bold: slot.Bold}
			src = f.sources[slot]
		}
	}
	return slot, src
}

func (f *Fonts) lookupLocked(size int32, slot frame.FontSlot) *faceEntry {
	if size <= 0 {
		return nil
	}
	slot, src := f.resolveLocked(slot)
	if src == nil {
		return nil
	}
	return f.entries[faceKey{slot: slot, size: size}]
}

func (f *Fonts) fallbackLocked(size int32, slot frame.FontSlot) []*faceEntry {
	k := faceKey{slot: slot, size: size}
	if fb, ok := f.fbCache[k]; ok {
		return fb
	}
	var out []*faceEntry
	if fb := f.faceLocked(size, frame.FontSlot{Bold: slot.Bold}); fb != nil {
		out = append(out, fb)
	}
	slots := make([]frame.FontSlot, 0, len(f.sources))
	for s := range f.sources {
		if s == slot || s == (frame.FontSlot{Bold: slot.Bold}) {
			continue
		}
		slots = append(slots, s)
	}
	sort.Slice(slots, func(i, j int) bool {
		a, b := slots[i], slots[j]
		if a.Family != b.Family {
			return a.Family < b.Family
		}
		if a.Bold != b.Bold {
			return b.Bold
		}
		if a.Italic != b.Italic {
			return a.Italic
		}
		return a.Light && !b.Light
	})
	for _, s := range slots {
		fb := f.faceLocked(size, s)
		if fb == nil {
			continue
		}
		seen := false
		for _, e := range out {
			if e == fb {
				seen = true
				break
			}
		}
		if !seen {
			out = append(out, fb)
		}
	}
	for i, fbSrc := range f.fallbacks {
		fb := f.fallbackFaceLocked(i, size, fbSrc)
		if fb == nil {
			continue
		}
		seen := false
		for _, e := range out {
			if e == fb {
				seen = true
				break
			}
		}
		if !seen {
			out = append(out, fb)
		}
	}
	f.fbCache[k] = out
	return out
}

func isDefaultIgnorable(r rune) bool {
	return (r >= 0xFE00 && r <= 0xFE0F) ||
		(r >= 0xE0100 && r <= 0xE01EF) ||
		r == 0x00AD || r == 0x200B || r == 0x200C || r == 0x200D ||
		r == 0x200E || r == 0x200F || r == 0xFEFF
}

func tofuAdvanceFixed(size int32) int32 {
	if size <= 0 {
		return 0
	}
	return size << 6
}

func tofuGlyph(size int32) Glyph {
	if size <= 0 {
		return Glyph{}
	}
	adv := size
	w := (size * 3) / 4
	if w < 2 {
		w = 2
	}
	pad := (adv - w) / 2
	top := -size
	if top >= 0 {
		top = -1
	}
	bottom := size / 8
	if bottom <= 0 {
		bottom = 1
	}
	h := bottom - top
	if h < 2 {
		h = 2
	}
	bounds := frame.Rect4(pad, top, pad+w, bottom)
	m := image.NewAlpha(image.Rect(0, 0, int(w), int(h)))
	stroke := int32(1)
	if size >= 32 {
		stroke = size / 16
	}
	for y := 0; y < int(h); y++ {
		row := m.Pix[y*m.Stride : y*m.Stride+int(w)]
		for x := 0; x < int(w); x++ {
			if x < int(stroke) || x >= int(w)-int(stroke) || y < int(stroke) || y >= int(h)-int(stroke) {
				row[x] = 0xFF
			}
		}
	}
	return Glyph{
		Mask:    m,
		Bounds:  bounds,
		Advance: adv,
		Ok:      true,
		Missing: true,
	}
}

func (f *Fonts) Glyph(size int32, r rune, slot frame.FontSlot) Glyph {
	if size <= 0 {
		return Glyph{}
	}
	if isDefaultIgnorable(r) {
		return Glyph{Ok: true}
	}
	f.mu.RLock()
	if e := f.lookupLocked(size, slot); e != nil {
		if g, ok := e.glyphs[r]; ok {
			if g.Missing {
				f.missingCount.Add(1)
			}
			f.mu.RUnlock()
			return g
		}
	}
	f.mu.RUnlock()
	f.mu.Lock()
	defer f.mu.Unlock()
	e := f.faceLocked(size, slot)
	if e == nil {
		return Glyph{}
	}
	if g, ok := e.glyphs[r]; ok {
		if g.Missing {
			f.missingCount.Add(1)
		}
		return g
	}
	g := renderGlyph(e.face, r)
	if !g.Ok {
		for _, fb := range f.fallbackLocked(size, slot) {
			if g = renderGlyph(fb.face, r); g.Ok {
				break
			}
		}
		if !g.Ok {
			if fb := f.lazyFallbackForRuneLocked(r, size); fb != nil {
				g = renderGlyph(fb.face, r)
			}
		}
	}
	if !g.Ok {
		e.missing[r] = struct{}{}
		f.missingCount.Add(1)
		tofu := tofuGlyph(size)
		e.glyphs[r] = tofu
		return tofu
	}
	e.glyphs[r] = g
	return g
}

func (f *Fonts) GlyphAdvance(size int32, r rune, slot frame.FontSlot) int32 {
	return f.Glyph(size, r, slot).Advance
}

func (f *Fonts) GlyphAdvanceFixed(size int32, r rune, slot frame.FontSlot) int32 {
	if size <= 0 {
		return 0
	}
	if isDefaultIgnorable(r) {
		return 0
	}
	f.mu.RLock()
	if e := f.lookupLocked(size, slot); e != nil {
		if adv, ok := e.adv[r]; ok {
			if _, bad := e.advMiss[r]; bad {
				f.missingCount.Add(1)
			}
			f.mu.RUnlock()
			return adv
		}
	}
	f.mu.RUnlock()
	f.mu.Lock()
	defer f.mu.Unlock()
	e := f.faceLocked(size, slot)
	if e == nil {
		return 0
	}
	if adv, ok := e.adv[r]; ok {
		if _, bad := e.advMiss[r]; bad {
			f.missingCount.Add(1)
		}
		return adv
	}
	adv, ok := e.face.GlyphAdvance(r)
	if !ok {
		for _, fb := range f.fallbackLocked(size, slot) {
			if adv, ok = fb.face.GlyphAdvance(r); ok {
				break
			}
		}
		if !ok {
			if fb := f.lazyFallbackForRuneLocked(r, size); fb != nil {
				adv, ok = fb.face.GlyphAdvance(r)
			}
		}
		if !ok {
			f.missingCount.Add(1)
			fixedAdv := tofuAdvanceFixed(size)
			e.advMiss[r] = struct{}{}
			e.adv[r] = fixedAdv
			return fixedAdv
		}
	}
	v := int32(adv)
	e.adv[r] = v
	return v
}

func (f *Fonts) MissingCount() int64 {
	return f.missingCount.Load()
}

func (f *Fonts) ResetMissingCount() {
	f.missingCount.Store(0)
}

func HasGlyph(face font.Face, r rune) bool {
	if face == nil {
		return false
	}
	_, _, _, _, ok := face.Glyph(fixed.Point26_6{}, r)
	return ok
}

func (f *Fonts) FaceForRune(size int32, r rune, slot frame.FontSlot) (font.Face, bool) {
	if size <= 0 {
		return nil, false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	e := f.faceLocked(size, slot)
	if e == nil {
		return nil, false
	}
	if HasGlyph(e.face, r) {
		return e.face, true
	}
	for _, fb := range f.fallbackLocked(size, slot) {
		if HasGlyph(fb.face, r) {
			return fb.face, true
		}
	}
	if fb := f.lazyFallbackForRuneLocked(r, size); fb != nil {
		if HasGlyph(fb.face, r) {
			return fb.face, true
		}
	}
	return nil, false
}

func (f *Fonts) LineMetrics(size int32, slot frame.FontSlot) (int32, int32, int32) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e := f.faceLocked(size, slot)
	if e == nil {
		return 0, 0, 0
	}
	m := e.face.Metrics()
	round := func(v fixed.Int26_6) int32 { return int32((int64(v) + 32) >> 6) }
	return round(m.Ascent), round(m.Descent), round(m.Height)
}

func (f *Fonts) XHeightFixed(size int32, slot frame.FontSlot) int32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	e := f.faceLocked(size, slot)
	if e == nil {
		return 0
	}
	return int32(e.face.Metrics().XHeight)
}

func renderGlyph(face font.Face, r rune) Glyph {
	dr, mask, maskp, advance, ok := face.Glyph(fixed.Point26_6{}, r)
	adv := int32((advance + 32) >> 6)
	if !ok {
		return Glyph{Advance: adv}
	}
	w, h := dr.Dx(), dr.Dy()
	g := Glyph{
		Bounds:  frame.Rect4(int32(dr.Min.X), int32(dr.Min.Y), int32(dr.Max.X), int32(dr.Max.Y)),
		Advance: adv,
		Ok:      true,
	}
	if w <= 0 || h <= 0 {
		return g
	}
	m := image.NewAlpha(image.Rect(0, 0, w, h))
	if am, isAlpha := mask.(*image.Alpha); isAlpha {
		mb := am.Bounds()
		for y := 0; y < h; y++ {
			my := y + maskp.Y
			if my < mb.Min.Y || my >= mb.Max.Y {
				continue
			}
			row := m.Pix[y*m.Stride : y*m.Stride+w]
			for x := 0; x < w; x++ {
				mx := x + maskp.X
				if mx < mb.Min.X || mx >= mb.Max.X {
					continue
				}
				row[x] = am.Pix[my*am.Stride+mx]
			}
		}
	} else {
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				_, _, _, a := mask.At(x+maskp.X, y+maskp.Y).RGBA()
				m.Pix[y*m.Stride+x] = byte(a >> 8)
			}
		}
	}
	g.Mask = m
	return g
}
