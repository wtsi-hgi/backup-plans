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

var Index = httpembed.HandleBuffer("index.html", indexHTML, 40134, time.Unix(1760973362, 0)) //nolint:gochecknoglobals,lll,mnd
