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
<<<<<<< HEAD
	uncompressedSize = 47440
	lastModifiedTime = 1763050493
=======
	uncompressedSize = 47082
	lastModifiedTime = 1762873734
>>>>>>> 0b78155 (Allow wildcards in fofns)
)

var Index = httpembed.HandleBuffer("index.html", indexHTML, uncompressedSize, time.Unix(lastModifiedTime, 0)) //nolint:gochecknoglobals,lll
