//go:build darwin && arm64

package splitbin

import _ "embed"

//go:embed splitd_darwin_arm64
var platformPlugin []byte
