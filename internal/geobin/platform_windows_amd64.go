//go:build windows && amd64

package geobin

import _ "embed"

//go:embed gost-geo-plugin_windows_amd64.exe
var platformPlugin []byte
