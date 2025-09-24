package server

import "strings"

type Summariser interface {
	Summary(string) (*DirSummary, error)
}

type TopLevelDir struct {
	parent   *TopLevelDir
	children map[string]Summariser
	summary  DirSummary
}

func newTopLevelDir(parent *TopLevelDir) *TopLevelDir {
	return &TopLevelDir{
		parent:   parent,
		children: make(map[string]Summariser),
		summary: DirSummary{
			Children: make(map[string]*DirSummary),
		},
	}
}

func (t *TopLevelDir) SetChild(name string, child Summariser) error {
	t.children[name] = child

	return t.Update()
}

func (t *TopLevelDir) Update() error {
	clear(t.summary.Children)
	t.summary.RuleSummaries = t.summary.RuleSummaries[:0]

	for name, child := range t.children {
		s, err := child.Summary("")
		if err != nil {
			return err
		}

		t.summary.MergeRules(s.RuleSummaries)
		t.summary.Children[name] = &DirSummary{
			RuleSummaries: s.RuleSummaries,
		}
	}

	if t.parent != nil {
		return t.parent.Update()
	}

	return nil
}

func (t *TopLevelDir) Summary(path string) (*DirSummary, error) {
	if path == "" {
		return &t.summary, nil
	}

	pos := strings.IndexByte(path, '/')

	child := t.children[path[:pos+1]]
	if child == nil {
		return nil, ErrNotFound
	}

	return child.Summary(path[pos+1:])
}
