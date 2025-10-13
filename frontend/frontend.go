//go:build !dev
// +build !dev

package frontend

import (
	_ "embed"
	"time"

	"vimagination.zapto.org/httpembed"
)

//go:embed index.html.gz
var indexHTML []byte

var Index = httpembed.HandleBuffer("index.html", indexHTML, 35446, time.Unix(1760342138, 0))
