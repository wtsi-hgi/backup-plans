package gitignore

import (
	"fmt"
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
	contents := strings.Split(string(file), "\n")
	g := &GitIgnore{
		IgnoreLines: contents,
		Matcher:     parser.CompileIgnoreLines(contents...),
	}

	return g, nil
}

// Returns subset slices ignore and keep of the input, dividing up each input
// path into files to ignore and files to backup
func (g *GitIgnore) Match(paths []string) (ignore []string, keep []string) {
	for _, rule := range paths {
		if g.Matcher.MatchesPath(rule) {
			ignore = append(ignore, rule)
		} else {
			keep = append(keep, rule)
		}
	}
	return
}

// Given a gitignore object, returns gitignore rules
func (g *GitIgnore) GetRules() ([]string, error) {
	if g.IgnoreLines == nil {
		return nil, fmt.Errorf("rules empty")
	}
	return g.IgnoreLines, nil
}

// Given a gitignore object, rules can be removed
func (g *GitIgnore) RemoveRules(rules []string) ([]string, error) {
	if g.IgnoreLines == nil {
		return nil, fmt.Errorf("rules empty")
	}

	removeRules := make(map[string]struct{})
	for _, r := range rules {
		removeRules[r] = struct{}{}
	}

	var newRules []string
	for _, rule := range g.IgnoreLines {
		_, exists := removeRules[rule]
		if !exists {
			newRules = append(newRules, rule)
		}
	}

	g.IgnoreLines = newRules
	g.Matcher = parser.CompileIgnoreLines(g.IgnoreLines...)
	return g.IgnoreLines, nil
}

// Given a gitignore object, rules can be added
func (g *GitIgnore) AddRules(rules []string) ([]string, error) {
	for _, r := range rules {
		exists := false
		for _, item := range g.IgnoreLines {
			if item == r {
				exists = true
				break
			}
		}
		if !exists {
			g.IgnoreLines = append(g.IgnoreLines, r)
		}
	}
	g.Matcher = parser.CompileIgnoreLines(g.IgnoreLines...)
	return g.IgnoreLines, nil
}
