package frontend

import (
	"net/http"
	"os"

	"vimagination.zapto.org/tsserver"
)

var fs = http.FileServerFS(tsserver.WrapFS(os.DirFS("./src")))

func Serve(w http.ResponseWriter, r *http.Request) {
	fs.ServeHTTP(w, r)
}
