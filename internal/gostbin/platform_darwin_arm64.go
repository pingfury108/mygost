//go:build darwin && arm64

package gostbin

import _ "embed"

//go:embed gost_darwin_arm64.tar.gz
var embeddedArchive []byte
