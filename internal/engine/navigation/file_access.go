package navigation

import (
	"errors"
	"net/url"
)

// ErrFileAccessDenied is returned when a remote origin attempts to navigate
// to or fetch a file:// URL. This is the default security policy to prevent
// local file access from remote web pages.
var ErrFileAccessDenied = errors.New("navigation: local file access denied from remote origin")

// CheckFileAccess determines whether a navigation or resource load to
// targetURL is allowed from a page currently at currentURL.
//
// The default policy:
//   - Non-file:// targets are always allowed.
//   - file:// targets are allowed only when currentURL is empty (initial
//     navigation) or when currentURL also uses the file:// scheme (same-scheme
//     navigation, e.g., clicking a link between local files).
//   - file:// targets from remote origins (https, http, data, blob, etc.)
//     are blocked with ErrFileAccessDenied.
//
// Parse errors for either URL are treated conservatively (allowing the
// navigation) so that a malformed URL does not create a security issue.
func CheckFileAccess(currentURL, targetURL string) error {
	target, err := url.Parse(targetURL)
	if err != nil {
		return nil
	}
	if target.Scheme != "file" {
		return nil
	}

	// No current page (initial navigation) — allowed.
	if currentURL == "" {
		return nil
	}

	current, err := url.Parse(currentURL)
	if err != nil {
		return nil
	}

	// Same-scheme access: a file:// page may navigate to other file:// URLs.
	if current.Scheme == "file" {
		return nil
	}

	return ErrFileAccessDenied
}
