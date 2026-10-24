//go:build darwin && amd64

package gostbin

import _ "embed"

//go:embed gost_darwin_amd64.tar.gz
var embeddedArchive []byte
