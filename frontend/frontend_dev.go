//go:build dev

package frontend

import (
	"net/http"
	"os"

	"vimagination.zapto.org/tsserver"
)

var Index = http.FileServerFS(tsserver.WrapFS(os.DirFS("./src")))
