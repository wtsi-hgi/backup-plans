package ruletree

import (
	"maps"
	"slices"
)

type wildcards struct {
	children map[string]*wildcards
	wildcard int64
}

func (r *RootDir) buildWildcards() {
	r.wildcards.children = map[string]*wildcards{}

	paths := slices.Collect(maps.Keys(r.directoryRules))

	slices.Sort(paths)

	for _, path := range paths {
		if wildcard, hasWildcard := r.directoryRules[path].Rules["*"]; hasWildcard {
			r.wildcards.Set(path, wildcard.ID())
		}
	}
}

func (w *wildcards) Child(name string) *wildcards {
	if w == nil {
		return nil
	}

	child := w.children[name]
	if child == nil && w.wildcard != 0 {
		if len(w.children) == 0 {
			return w
		}

		return &wildcards{wildcard: w.wildcard}
	}

	return child
}

func (w *wildcards) Wildcard() int64 {
	if w == nil {
		return 0
	}

	return w.wildcard
}

func (w *wildcards) Set(path string, wildcard int64) {
	curr := w

	for part := range pathParts(path[1:]) {
		next, ok := curr.children[part]
		if !ok {
			next = &wildcards{children: map[string]*wildcards{}, wildcard: curr.wildcard}
			curr.children[part] = next
		}

		curr = next
	}

	curr.wildcard = wildcard
}
