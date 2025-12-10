package ruletree

import (
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/wrstat-ui/summary/group"
)

const processRules int64 = math.MinInt64

type dirState uint8

const (
	rulesChanged dirState = 1 << iota
	hasChildWithRules
	parentRulesChanged
	hasChildWithChangedRules
)

type ruleState uint8

const (
	simpleWildcard ruleState = 1 << iota
	simplePaths
	complexWildcardWithPrefix
	complexWildcardWithSuffix

	noRules ruleState = 0
)

func (r *RootDir) generateStatemachineFor(mount string, paths []string) (group.StateMachine[int64], error) {
	rules := r.buildDirRules(mount, paths)

	return group.NewStatemachine(rules)
}

type dirTreeRule struct {
	dir   dirState
	rules ruleState
}

type dirTree struct {
	children map[string]*dirTree
	dirTreeRule
}

func newDirTree(dirTreeRule dirTreeRule) *dirTree {
	return &dirTree{
		children:    make(map[string]*dirTree),
		dirTreeRule: dirTreeRule,
	}
}

func (d *dirTree) set(path string, rs ruleState, changed bool) {
	curr := d

	for part := range pathParts(path[1:]) {
		next, ok := curr.children[part]
		if !ok {
			if changed && rs == 0 {
				curr.dir |= rulesChanged

				break
			}

			var new dirTreeRule

			if curr.dir&rulesChanged != 0 || curr.dir&parentRulesChanged != 0 {
				new.dir |= parentRulesChanged
			}

			next = newDirTree(new)
			curr.children[part] = next
		}

		curr.dir |= hasChildWithRules

		if changed {
			curr.dir |= hasChildWithChangedRules
		}

		curr = next
	}

	if changed {
		curr.dir |= rulesChanged
	}

	curr.rules = rs
}

func (r *RootDir) buildDirRules(mount string, paths []string) []group.PathGroup[int64] {
	dirs := slices.Collect(maps.Keys(r.directoryRules))

	slices.Sort(dirs)

	root := newDirTree(dirTreeRule{rules: simpleWildcard})
	root.set(mount, simpleWildcard, false)
	root.set("/", simpleWildcard, true)

	for _, dir := range dirs {
		if !strings.HasPrefix(dir, mount) {
			continue
		}

		root.set(dir, r.ruleState(dir), paths == nil || slices.Contains(paths, dir))
	}

	return r.buildRules(root, "/", nil, 0)
}

func (r *RootDir) ruleState(dir string) ruleState {
	var rs ruleState

	drs := r.directoryRules[dir]

	if drs == nil {
		return 0
	}

	for match := range drs.Rules {
		if match == "*" {
			rs |= simpleWildcard
		} else if strings.Contains(match, "*") {
			if match[0] == '*' {
				rs |= complexWildcardWithSuffix
			} else {
				rs |= complexWildcardWithPrefix
			}
		} else {
			rs |= simplePaths
		}
	}

	return rs
}

func (r *RootDir) buildRules(d *dirTree, path string, rules []group.PathGroup[int64], wildcard int64) []group.PathGroup[int64] {
	drs, ok := r.directoryRules[path]
	if ok {
		if wc, ok := drs.Rules["*"]; ok {
			wildcard = wc.ID()
		}

		for match, rule := range drs.Rules {
			rules = addRule(rules, path+match, rule.ID())
		}
	}

	switch d.rules {
	case noRules:
		rules = addNoRuleRules(rules, path, d.dir, wildcard)
	case simpleWildcard:
		rules = addSimpleWildcardRules(rules, path, d.dir, wildcard)
	case simplePaths:
		rules = addRule(rules, path, processRules)
	case simpleWildcard | simplePaths:
		rules = addSimpleRules(rules, path, d.dir, wildcard)
	case complexWildcardWithPrefix, complexWildcardWithSuffix, complexWildcardWithPrefix | simplePaths, complexWildcardWithSuffix | simplePaths:
		rules = addComplexRules(rules, path, d.dir, d.rules, wildcard, drs.Rules)
	default:
		rules = addComplexWithWildcardRules(rules, path, d.dir, d.rules, wildcard, drs.Rules)
	}

	for part, child := range d.children {
		rules = r.buildRules(child, path+part, rules, wildcard)
	}

	return rules
}

func addRule(rules []group.PathGroup[int64], path string, rule int64) []group.PathGroup[int64] {
	return append(rules, group.PathGroup[int64]{
		Path:  []byte(path),
		Group: &rule,
	})
}

func addNoRuleRules(rules []group.PathGroup[int64], path string, dirState dirState, wildcard int64) []group.PathGroup[int64] {
	process := processRules

	if dirState&rulesChanged != 0 && dirState&hasChildWithRules == 0 && dirState&parentRulesChanged == 0 {
		process = wildcard
	}

	return addRule(rules, path, process)
}

func addSimpleWildcardRules(rules []group.PathGroup[int64], path string, dirState dirState, wildcard int64) []group.PathGroup[int64] {
	if dirState&rulesChanged != 0 {
		if dirState&hasChildWithRules != 0 {
			return addRule(addRule(rules, path, processRules), path+"*/", wildcard)
		} else {
			return addRule(rules, path, wildcard)
		}
	} else if dirState&hasChildWithChangedRules != 0 {
		return addRule(addRule(rules, path, processRules), path+"*/", -wildcard)
	}

	return addRule(rules, path, -wildcard)
}

func addSimpleRules(rules []group.PathGroup[int64], path string, dirState dirState, wildcard int64) []group.PathGroup[int64] {
	if dirState&rulesChanged != 0 {
		return addRule(addRule(rules, path, processRules), path+"*/", wildcard)
	} else if dirState&hasChildWithChangedRules != 0 {
		return addRule(addRule(rules, path, processRules), path+"*/", -wildcard)
	}

	return addRule(rules, path, -wildcard)
}

func addComplexRules(rules []group.PathGroup[int64], path string, dirState dirState, ruleState ruleState, wildcard int64, rs map[string]*db.Rule) []group.PathGroup[int64] {
	rules = addRule(rules, path, processRules)

	if dirState&rulesChanged == 0 && dirState&parentRulesChanged == 0 {
		return addRule(rules, path+"*/", -wildcard)
	}

	return addComplexChildRules(rules, path, ruleState, wildcard, rs)
}

func addComplexChildRules(rules []group.PathGroup[int64], path string, ruleState ruleState, wildcard int64, rs map[string]*db.Rule) []group.PathGroup[int64] {
	if ruleState&complexWildcardWithSuffix != 0 {
		return addRule(rules, path+"*/", processRules)
	} else if ruleState&simpleWildcard != 0 {
		addRule(rules, path+"*/", -wildcard)
	}

	todo := map[string]struct{}{}

	for match := range rs {
		if pos := strings.IndexByte(match, '*'); pos > 0 {
			todo[match[:pos]] = struct{}{}
		}
	}

	for part := range todo {
		addRule(rules, path+part+"*/", processRules)
	}

	return rules
}

func addComplexWithWildcardRules(rules []group.PathGroup[int64], path string, dirState dirState, ruleState ruleState, wildcard int64, rs map[string]*db.Rule) []group.PathGroup[int64] {
	if dirState&rulesChanged == 0 {
		if dirState&hasChildWithChangedRules != 0 {
			return addRule(addRule(rules, path, processRules), path+"*/", -wildcard)
		} else {
			return addRule(rules, path, -wildcard)
		}
	}

	rules = addRule(rules, path, processRules)

	return addComplexChildRules(rules, path, ruleState, wildcard, rs)
}
