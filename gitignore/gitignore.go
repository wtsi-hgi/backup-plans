package gitignore

import (
	"fmt"
	"io"
	"slices"
	"strings"

	parser "github.com/sabhiram/go-gitignore"
)

type GitIgnore struct {
	Matcher     *parser.GitIgnore
	IgnoreLines []string
}

// New returns a gitignore object given git ignore data.
func New(r io.Reader) (gi *GitIgnore, err error) {
	gitIngoreData, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	contents := strings.Split(string(gitIngoreData), "\n")
	contentsFiltered := make([]string, 0, len(contents))
	for _, line := range contents {
		if line == "" {
			continue
		}
		contentsFiltered = append(contentsFiltered, line)
	}

	g := &GitIgnore{
		IgnoreLines: contentsFiltered,
		Matcher:     parser.CompileIgnoreLines(contentsFiltered...),
	}

	return g, nil
}

// Match returns subset slices ignore and keep of the input, dividing up each
// input path into files to ignore and files to backup.
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

// GetRules returns our gitignore rules.
func (g *GitIgnore) GetRules() ([]string, error) {
	if g.IgnoreLines == nil {
		return nil, fmt.Errorf("rules empty")
	}

	return g.IgnoreLines, nil
}

// RemoveRules removes the given rules from our set of rules.
func (g *GitIgnore) RemoveRules(rules []string) ([]string, error) {
	if g.IgnoreLines == nil || len(rules) == 0 {
		return g.IgnoreLines, nil
	}

	removeRules := make(map[string]struct{})
	for _, r := range rules {
		removeRules[r] = struct{}{}
	}

	newRules := make([]string, 0, len(g.IgnoreLines))

	for _, rule := range g.IgnoreLines {
		if _, exists := removeRules[rule]; !exists {
			newRules = append(newRules, rule)
		}
	}

	g.IgnoreLines = newRules
	g.Matcher = parser.CompileIgnoreLines(g.IgnoreLines...)

	return g.IgnoreLines, nil
}

// AddRules adds the given rules.
func (g *GitIgnore) AddRules(rules []string) ([]string, error) {
	for _, r := range rules {
		if exists := slices.Contains(g.IgnoreLines, r); exists {
			continue
		}

		g.IgnoreLines = append(g.IgnoreLines, r)
	}

	g.Matcher = parser.CompileIgnoreLines(g.IgnoreLines...)

	return g.IgnoreLines, nil
}

// AddRulesAt adds the given rule at the specified index. Returns an error if
// the index is too high.
func (g *GitIgnore) AddRuleAt(rule string, index int) ([]string, error) {
	if exists := slices.Contains(g.IgnoreLines, rule); exists {
		return g.IgnoreLines, nil
	}

	var err error

	g.IgnoreLines, err = insert(g.IgnoreLines, index, rule)
	if err != nil {
		return nil, err
	}

	g.Matcher = parser.CompileIgnoreLines(g.IgnoreLines...)

	return g.IgnoreLines, nil
}

func insert(s []string, i int, val string) ([]string, error) {
	if i < 0 || i > len(s) {
		return nil, fmt.Errorf("invalid index %+v", i)
	}

	s = append(s[:i+1], s[i:]...)
	s[i] = val

	return s, nil
}
