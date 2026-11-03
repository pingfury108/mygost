//go:build windows && amd64

package splitbin

import _ "embed"

//go:embed splitd_windows_amd64.exe
var platformPlugin []byte
