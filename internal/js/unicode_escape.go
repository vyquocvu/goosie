package js

import "strconv"

// replaceGojaMaxCodePointEscape works around a Goja lexer panic on the valid
// JavaScript string escape \u{10FFFF}. The equivalent UTF-16 surrogate escapes
// produce the same string value and avoid changing escaped backslashes or code
// outside string literals.
func replaceGojaMaxCodePointEscape(src string) string {
	var out []byte
	quote := byte(0)
	for i := 0; i < len(src); {
		c := src[i]
		if quote == 0 {
			if c == '\'' || c == '"' {
				quote = c
			}
			if out != nil {
				out = append(out, c)
			}
			i++
			continue
		}
		if c == quote {
			quote = 0
			if out != nil {
				out = append(out, c)
			}
			i++
			continue
		}
		if c == '\\' {
			if i+2 < len(src) && src[i+1] == 'u' && src[i+2] == '{' {
				end := i + 3
				for end < len(src) && src[end] != '}' && end-i <= 10 {
					end++
				}
				if end < len(src) && src[end] == '}' {
					if value, err := strconv.ParseUint(src[i+3:end], 16, 32); err == nil && value == 0x10FFFF {
						if out == nil {
							out = make([]byte, 0, len(src)+8)
							out = append(out, src[:i]...)
						}
						out = append(out, `\uDBFF\uDFFF`...)
						i = end + 1
						continue
					}
				}
			}
			if out != nil {
				out = append(out, c)
			}
			i++
			if i < len(src) {
				if out != nil {
					out = append(out, src[i])
				}
				i++
			}
			continue
		}
		if out != nil {
			out = append(out, c)
		}
		i++
	}
	if out == nil {
		return src
	}
	return string(out)
}
