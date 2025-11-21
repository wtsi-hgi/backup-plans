//go:build !dev
// +build !dev

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
	uncompressedSize = 51025
	lastModifiedTime = 1763741012
)

var Index = httpembed.HandleBuffer("index.html", indexHTML, uncompressedSize, time.Unix(lastModifiedTime, 0)) //nolint:gochecknoglobals,lll
