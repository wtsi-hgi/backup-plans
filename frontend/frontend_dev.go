//go:build dev

package frontend

import (
	"io"
	"net/http"
	"os"

	"vimagination.zapto.org/tsserver"
)

var Index = http.FileServerFS(tsserver.WrapFSWithErrorHandler(os.DirFS("./src"), func(w io.Writer, err error) {
	io.WriteString(w, err.Error())
}))
