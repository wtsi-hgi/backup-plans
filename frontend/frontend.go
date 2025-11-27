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
<<<<<<< HEAD
	uncompressedSize = 52736
	lastModifiedTime = 1764263373
=======
	uncompressedSize = 51839
	lastModifiedTime = 1764250278
>>>>>>> c24f4c4 (Allow relative paths in FOFNs)
)

var Index = httpembed.HandleBuffer("index.html", indexHTML, uncompressedSize, time.Unix(lastModifiedTime, 0)) //nolint:gochecknoglobals,lll
