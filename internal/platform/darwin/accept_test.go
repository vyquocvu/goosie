//go:build darwin && cgo

package darwin

import (
	"reflect"
	"testing"
)

func TestParseFileAcceptSeparatesExtensionsAndMIMETypes(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want fileAccept
	}{
		{name: "empty", raw: "", want: fileAccept{}},
		{name: "extensions", raw: ".txt,.PNG", want: fileAccept{Extensions: []string{"txt", "png"}}},
		{name: "mime wildcard", raw: "image/*", want: fileAccept{MIMETypes: []string{"image/*"}}},
		{name: "mime concrete", raw: "image/png", want: fileAccept{MIMETypes: []string{"image/png"}}},
		{name: "mixed", raw: "image/png, .jpg", want: fileAccept{Extensions: []string{"jpg"}, MIMETypes: []string{"image/png"}}},
		{name: "whitespace and case", raw: " TEXT/Plain , Audio/* ", want: fileAccept{MIMETypes: []string{"text/plain", "audio/*"}}},
		{name: "any type", raw: "*/*", want: fileAccept{MIMETypes: []string{"*/*"}}},
		{name: "duplicates collapse", raw: "IMAGE/PNG,.png,image/png", want: fileAccept{Extensions: []string{"png"}, MIMETypes: []string{"image/png"}}},
		{name: "bare extension ignored", raw: "txt", want: fileAccept{}},
		{name: "malformed mime ignored", raw: "image,/png,image/,image/*/*,  ,", want: fileAccept{}},
		{name: "compound extension kept", raw: ".tar.gz", want: fileAccept{Extensions: []string{"tar.gz"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseFileAccept(tc.raw)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseFileAccept(%q) = %+v, want %+v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestFileAcceptDecidesWhetherThePanelFilters(t *testing.T) {
	for _, raw := range []string{"", "txt", "*/*", "application/*"} {
		if got := parseFileAccept(raw); got.filters() {
			t.Errorf("parseFileAccept(%q).filters() = true, want unfiltered", raw)
		}
	}
	for _, raw := range []string{".png", "image/*", "image/png", "image/*,.jpg"} {
		if got := parseFileAccept(raw); !got.filters() {
			t.Errorf("parseFileAccept(%q).filters() = false, want filtered", raw)
		}
	}
}

func TestFileAcceptNativeFilterListsWhatThePanelCanApply(t *testing.T) {
	tests := []struct {
		raw            string
		wantExtensions string
		wantMIMETypes  string
	}{
		{raw: "", wantExtensions: "", wantMIMETypes: ""},
		{raw: ".txt,.PNG", wantExtensions: "txt,png", wantMIMETypes: ""},
		{raw: "image/*", wantExtensions: "", wantMIMETypes: "image/*"},
		{raw: "image/png,.jpg", wantExtensions: "jpg", wantMIMETypes: "image/png"},
		{raw: "*/*", wantExtensions: "", wantMIMETypes: ""},
		{raw: "application/*,.zip", wantExtensions: "", wantMIMETypes: ""},
	}
	for _, tc := range tests {
		extensions, mimeTypes := parseFileAccept(tc.raw).nativeFilter()
		if extensions != tc.wantExtensions || mimeTypes != tc.wantMIMETypes {
			t.Errorf("parseFileAccept(%q).nativeFilter() = %q,%q, want %q,%q",
				tc.raw, extensions, mimeTypes, tc.wantExtensions, tc.wantMIMETypes)
		}
	}
}
