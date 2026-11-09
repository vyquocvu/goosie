package paint

// bidiMirror maps each Unicode character with Bidi_Mirrored=Yes to its
// mirrored counterpart. Chromium 1.63 flips bracket glyphs in RTL context
// (scripts/verify-br087-bidi-residuals.js: RTL "(" has the same ink shape as
// LTR ")"). The map covers the ASCII, Latin-1 Supplement, and CJK bracket
// pairs the engine's test corpus exercises; the full Unicode list is larger
// but these are the ones browsers mirror in practice.
var bidiMirror = map[rune]rune{
	// ASCII brackets
	'(': ')', ')': '(',
	'[': ']', ']': '[',
	'{': '}', '}': '{',
	'<': '>', '>': '<',
	// Latin-1 and Latin Extended
	'«': '»', '»': '«',
	'‹': '›', '›': '‹',
	// Mathematical brackets
	'⟨': '⟩', '⟩': '⟨',
	'⟪': '⟫', '⟫': '⟪',
	'⟬': '⟭', '⟭': '⟬',
	'⟮': '⟯', '⟯': '⟮',
	// CJK brackets
	'「': '」', '」': '「',
	'『': '』', '』': '『',
	'【': '】', '】': '【',
	'〔': '〕', '〕': '〔',
	'〖': '〗', '〗': '〖',
	'〘': '〙', '〙': '〘',
	'〚': '〛', '〛': '〚',
	// Fullwidth forms
	'（': '）', '）': '（',
	'［': '］', '］': '［',
	'｛': '｝', '｝': '｛',
	'＜': '＞', '＞': '＜',
	'｟': '｠', '｠': '｟',
	// Corner and white corner brackets (alternate forms)
	'⌈': '⌉', '⌉': '⌈',
	'⌊': '⌋', '⌋': '⌊',
}

// mirrorRune returns the bidi-mirrored form of r if one exists and the
// context is RTL (odd bidi level). Otherwise it returns r unchanged.
func mirrorRune(r rune, level byte) rune {
	if level&1 == 0 {
		return r
	}
	if m, ok := bidiMirror[r]; ok {
		return m
	}
	return r
}
