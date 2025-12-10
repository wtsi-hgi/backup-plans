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
	rules map[string]*db.Rule
}

type RuleTree struct {
	children map[string]*RuleTree
	dirTreeRule
}

func NewRuleTree(dirTreeRule dirTreeRule) *RuleTree {
	if dirTreeRule.rules == nil {
		dirTreeRule.rules = map[string]*db.Rule{}
	}

	return &RuleTree{
		children:    make(map[string]*RuleTree),
		dirTreeRule: dirTreeRule,
	}
}

func (r *RuleTree) set(path string, rules map[string]*db.Rule, changed bool) {
	curr := r

	for part := range pathParts(path[1:]) {
		next, ok := curr.children[part]
		if !ok {
			if changed && len(rules) == 0 {
				curr.dir |= rulesChanged

				break
			}

			var new dirTreeRule

			if curr.dir&rulesChanged != 0 || curr.dir&parentRulesChanged != 0 {
				new.dir |= parentRulesChanged
			}

			next = NewRuleTree(new)
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

	curr.rules = maps.Clone(rules)
}

func (r *RuleTree) Get(path string) *RuleTree {
	curr := r
	for part := range pathParts(path[1:]) {
		next, ok := curr.children[part]
		if !ok {
			return nil
		}

		curr = next
	}

	return curr
}

func (r *RuleTree) Canon() {
	for _, child := range r.children {
		child.Canon()
	}

	if r.resolveOverrides() {
		r.recurseResolveSlashes()
	} else {
		r.resolveSlashes()
	}
}

func (r *RuleTree) resolveOverrides() bool {
	changed := r.dir&rulesChanged != 0

	hasOverride := false

	for match, rule := range r.rules {
		if rule == nil || !rule.Override {
			continue
		}

		hasOverride = true

		for _, child := range r.children {
			child.setOverride(match, rule, changed)
		}

		if !strings.HasPrefix(match, "*") {
			r.setOverride("*/"+match, rule, changed)
		}
	}

	return hasOverride
}

func (r *RuleTree) setOverride(match string, rule *db.Rule, changed bool) {
	if _, ok := r.rules[match]; ok {
		slash := strings.IndexByte(match, '/')
		wildcard := strings.IndexByte(match, '*')

		if wildcard < 0 || slash >= 0 && wildcard < slash {
			return
		}
	} else {
		r.rules[match] = rule

		if changed {
			r.dir |= rulesChanged
		}
	}

	for _, child := range r.children {
		child.setOverride(match, rule, changed)
	}
}

func (r *RuleTree) recurseResolveSlashes() {
	for _, child := range r.children {
		child.recurseResolveSlashes()
	}

	r.resolveSlashes()
}

func (r *RuleTree) resolveSlashes() {
	changed := r.dir&rulesChanged != 0

	for match, rule := range r.rules {
		preWildcard, _, _ := strings.Cut(match, "*")
		slash := strings.LastIndexByte(preWildcard, '/')

		if slash < 0 {
			continue
		}

		delete(r.rules, match)

		curr := r

		for part := range pathParts(match[:slash+1]) {
			next, ok := curr.children[part]
			if !ok {
				var new dirTreeRule

				if changed || curr.dir&parentRulesChanged != 0 {
					new.dir |= parentRulesChanged
				}

				next = NewRuleTree(new)
				curr.children[part] = next
			}

			curr.dir |= hasChildWithRules

			if changed {
				curr.dir |= hasChildWithChangedRules
			}

			curr = next
		}

		newMatch := match[slash+1:]

		if _, ok := curr.rules[newMatch]; !ok {
			curr.rules[newMatch] = rule
		}
	}
}

var basicWildcard = map[string]*db.Rule{"*": nil}

func (r *RootDir) buildDirRules(mount string, paths []string) []group.PathGroup[int64] {
	dirs := slices.Collect(maps.Keys(r.directoryRules))

	slices.Sort(dirs)

	root := NewRuleTree(dirTreeRule{})
	root.set(mount, basicWildcard, false)
	root.set("/", basicWildcard, true)

	for _, dir := range dirs {
		if !strings.HasPrefix(dir, mount) {
			continue
		}

		root.set(dir, r.directoryRules[dir].Rules, paths == nil || slices.Contains(paths, dir))
	}

	root.Canon()

	return buildRules(root, "/", nil, 0)
}

func buildRules(d *RuleTree, path string, rules []group.PathGroup[int64], wildcard int64) []group.PathGroup[int64] {
	if len(d.rules) > 0 {
		if wc, ok := d.rules["*"]; ok {
			wildcard = wc.ID()
		}

		for match, rule := range d.rules {
			rules = addRule(rules, path+match, rule.ID())
		}
	}

	switch ruleState := getRuleState(d.rules); ruleState {
	case noRules:
		rules = addNoRuleRules(rules, path, d.dir, wildcard)
	case simpleWildcard:
		rules = addSimpleWildcardRules(rules, path, d.dir, wildcard)
	case simplePaths:
		rules = addRule(rules, path, processRules)
	case simpleWildcard | simplePaths:
		rules = addSimpleRules(rules, path, d.dir, wildcard)
	case complexWildcardWithPrefix, complexWildcardWithSuffix, complexWildcardWithPrefix | simplePaths, complexWildcardWithSuffix | simplePaths:
		rules = addComplexRules(rules, path, d.dir, ruleState, wildcard, d.rules)
	default:
		rules = addComplexWithWildcardRules(rules, path, d.dir, ruleState, wildcard, d.rules)
	}

	for part, child := range d.children {
		rules = buildRules(child, path+part, rules, wildcard)
	}

	return rules
}

func getRuleState(rules map[string]*db.Rule) ruleState {
	if len(rules) == 0 {
		return 0
	}

	var rs ruleState

	for match := range rules {
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
		rules = addRule(rules, path+"*/", -wildcard)
	}

	todo := map[string]int64{}

	for match, r := range rs {
		if pos := strings.IndexByte(match, '*'); pos > 0 {
			newMatch := match[:pos]

			if _, ok := todo[newMatch]; ok || pos+1 < len(match) {
				todo[newMatch] = processRules
			} else {
				todo[newMatch] = r.ID()
			}
		}
	}

	for part, id := range todo {
		rules = addRule(rules, path+part+"*/", id)
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
