package style

import (
	"strings"

	"github.com/vyquocvu/goosie/internal/css"
)

func ResolveGridTrackLength(s string, fontSize, pctBase, viewportW, viewportH float32) (float32, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if i := strings.IndexByte(s, '('); i > 0 && strings.HasSuffix(s, ")") {
		name := strings.ToLower(strings.TrimSpace(s[:i]))
		switch name {
		case "calc", "min", "max", "clamp":
			inner := strings.TrimSpace(s[i+1 : len(s)-1])
			return css.EvalFuncWithBaseAndRem(name, inner, fontSize, 16, viewportW, viewportH, pctBase)
		}
	}
	return css.EvalFuncWithBaseAndRem("calc", s, fontSize, 16, viewportW, viewportH, pctBase)
}
