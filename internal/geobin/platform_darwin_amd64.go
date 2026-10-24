//go:build darwin && amd64

package geobin

import _ "embed"

//go:embed gost-geo-plugin_darwin_amd64
var platformPlugin []byte
