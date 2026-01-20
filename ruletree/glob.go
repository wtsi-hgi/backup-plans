package ruletree

import (
	"path/filepath"
	"slices"
	"strings"

	"vimagination.zapto.org/tree"
)

func (r *RootDir) GlobPath(path string) []string {
	res := appendAll(nil, "/", r.globPath(path))

	slices.Sort(res)

	return res
}

func (r *RootDir) globPath(path string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.glob(strings.TrimPrefix(path, "/"))
}

func (r *RootDir) GlobPaths(paths ...string) []string {
	var res []string

	for _, path := range paths {
		res = appendAll(res, "/", r.globPath(path))
	}

	slices.Sort(res)

	return slices.Compact(res)
}

func (t *topLevelDir) glob(match string) []string {
	if match == "" {
		return []string{""}
	}

	pattern, rest := splitMatch(match)

	var res []string

	for dir, child := range t.children {
		if globMatch(pattern, dir) {
			res = appendAll(res, dir, child.glob(rest))
		}
	}

	return res
}

func globMatch(pattern, dir string) bool {
	m, _ := filepath.Match(strings.TrimSuffix(pattern, "/"), strings.TrimSuffix(dir, "/")) //nolint:errcheck

	return m && strings.HasSuffix(pattern, "/") == strings.HasSuffix(dir, "/")
}

func splitMatch(match string) (string, string) {
	pos := strings.IndexByte(match, '/')
	if pos == -1 {
		return match, ""
	}

	return match[:pos+1], match[pos+1:]
}

func appendAll(res []string, dir string, children []string) []string {
	for _, child := range children {
		res = append(res, dir+child)
	}

	return res
}

func (r *ruleOverlay) glob(match string) []string {
	if match == "" {
		return []string{""}
	}

	pattern, rest := splitMatch(match)

	var res []string

	for dir, child := range r.lower.Children() {
		if globMatch(pattern, dir) { //nolint:nestif
			if !strings.HasSuffix(dir, "/") {
				res = append(res, dir)
			} else {
				ro := &ruleOverlay{child.(*tree.MemTree), nil} //nolint:errcheck,forcetypeassert
				res = appendAll(res, dir, ro.glob(rest))
			}
		}
	}

	return res
}
