package gitignore

import (
	parser "github.com/sabhiram/go-gitignore"
)

type gi struct {
	Matcher *parser.GitIgnore
}

// Returns subset slices ignore and keep of the input, dividing up each input
// path into files to ignore and files to backup
func (g *gi) Match(paths []string) (ignore []string, keep []string) {
	return nil, nil
}

// Given a gitIgnore filepath, returns a gitignore scheme
func New(path string) (gi gi, err error) {
	matcher := parser.CompileIgnoreLines(path)
	gi.Matcher = matcher
	return gi, nil
}
