//go:build darwin && amd64

package splitbin

import _ "embed"

//go:embed splitd_darwin_amd64
var platformPlugin []byte
