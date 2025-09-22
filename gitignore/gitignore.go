package gitignore

import (
	"os"
	"strings"

	parser "github.com/sabhiram/go-gitignore"
)

type GitIgnore struct {
	Matcher     *parser.GitIgnore
	IgnoreLines []string
}

// Given a gitIgnore filepath, returns a gitignore object.
func New(path string) (gi *GitIgnore, err error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	g := &GitIgnore{}
	g.Matcher = parser.CompileIgnoreLines(string(file))
	g.IgnoreLines = strings.Split(string(file), "\n")

	return g, nil
}

// Returns subset slices ignore and keep of the input, dividing up each input
// path into files to ignore and files to backup
func (g *GitIgnore) Match(paths []string) (ignore []string, keep []string) {
	return nil, nil
}

// Given a gitignore object, returns gitignore rules
func (g *GitIgnore) GetRules() ([]string, error) {
	return nil, nil
}

// Given a gitignore object, rules can be removed
func (g *GitIgnore) RemoveRules([]string) error {
	return nil
}

// Given a gitignore object, rules can be added
func (g *GitIgnore) AddRules([]string) error {
	return nil
}
