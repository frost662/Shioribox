package main

import (
	"bytes"
	"unicode/utf8"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// decodeText turns a text file of unknown encoding into UTF-8. Japanese guides are often Shift-JIS.
func decodeText(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}):
		return string(b[3:])
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE}) || bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		if out, _, err := transform.Bytes(unicode.BOMOverride(unicode.UTF8.NewDecoder()), b); err == nil {
			return string(out)
		}
	}
	if utf8.Valid(b) {
		return string(b)
	}
	if out, _, err := transform.Bytes(japanese.ShiftJIS.NewDecoder(), b); err == nil {
		return string(out)
	}
	return string(bytes.ToValidUTF8(b, []byte("?")))
}
