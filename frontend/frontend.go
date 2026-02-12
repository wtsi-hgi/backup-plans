//go:build !dev

package frontend

// File automatically generated with ./embed.sh

import (
	_ "embed"
	"time"

	"vimagination.zapto.org/httpembed"
)

//go:embed index.html.gz
var indexHTML []byte

const (
	uncompressedSize = 285430
	lastModifiedTime = 1770905011
)

var Index = httpembed.HandleBuffer("index.html", indexHTML, uncompressedSize, time.Unix(lastModifiedTime, 0)) //nolint:gochecknoglobals,lll
