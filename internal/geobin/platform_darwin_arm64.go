//go:build darwin && arm64

package geobin

import _ "embed"

//go:embed gost-geo-plugin_darwin_arm64
var platformPlugin []byte
