//go:build windows && amd64

package gostbin

import _ "embed"

//go:embed gost_windows_amd64.zip
var embeddedArchive []byte
