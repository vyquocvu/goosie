//go:build darwin && cgo

package darwin

import "strings"

type fileAccept struct {
	Extensions []string
	MIMETypes  []string
}

func parseFileAccept(raw string) fileAccept {
	var accept fileAccept
	seenExtension := make(map[string]bool)
	seenMIME := make(map[string]bool)
	for _, token := range strings.Split(raw, ",") {
		token = strings.ToLower(strings.TrimSpace(token))
		if token == "" {
			continue
		}
		if strings.HasPrefix(token, ".") {
			extension := token[1:]
			if !validAcceptExtension(extension) || seenExtension[extension] {
				continue
			}
			seenExtension[extension] = true
			accept.Extensions = append(accept.Extensions, extension)
			continue
		}
		if !validAcceptMIME(token) || seenMIME[token] {
			continue
		}
		seenMIME[token] = true
		accept.MIMETypes = append(accept.MIMETypes, token)
	}
	return accept
}

func (a fileAccept) filters() bool {
	if len(a.MIMETypes) > 0 {
		for _, mime := range a.MIMETypes {
			if !representableAcceptMIME(mime) {
				return false
			}
		}
		return true
	}
	return len(a.Extensions) > 0
}

func (a fileAccept) nativeFilter() (extensions, mimeTypes string) {
	if !a.filters() {
		return "", ""
	}
	return strings.Join(a.Extensions, ","), strings.Join(a.MIMETypes, ",")
}

func representableAcceptMIME(mime string) bool {
	slash := strings.IndexByte(mime, '/')
	if slash < 0 {
		return false
	}
	mediaType, subtype := mime[:slash], mime[slash+1:]
	if subtype != "*" {
		return mediaType != "*"
	}
	switch mediaType {
	case "image", "audio", "video", "text":
		return true
	default:
		return false
	}
}

func validAcceptExtension(extension string) bool {
	if extension == "" {
		return false
	}
	for _, r := range extension {
		if r == '/' || r == '*' || isASCIISpace(r) {
			return false
		}
	}
	return true
}

func validAcceptMIME(token string) bool {
	slash := strings.IndexByte(token, '/')
	if slash <= 0 || slash == len(token)-1 || strings.IndexByte(token[slash+1:], '/') >= 0 {
		return false
	}
	mediaType, subtype := token[:slash], token[slash+1:]
	if mediaType == "*" && subtype != "*" {
		return false
	}
	for _, part := range []string{mediaType, subtype} {
		for _, r := range part {
			if !validAcceptMIMERune(r) {
				return false
			}
		}
	}
	return true
}

func validAcceptMIMERune(r rune) bool {
	switch {
	case r == '*' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
		return true
	case r == '!' || r == '#' || r == '$' || r == '&' || r == '^' || r == '_' ||
		r == '.' || r == '+' || r == '-':
		return true
	default:
		return false
	}
}

func isASCIISpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\r' || r == '\n' || r == '\f'
}
