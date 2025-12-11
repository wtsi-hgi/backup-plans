package ruletree

import (
	"iter"
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
	RulesChanged dirState = 1 << iota
	HasChildWithRules
	ParentRulesChanged
	HasChildWithChangedRules
)

type ruleState uint8

const (
	SimpleWildcard ruleState = 1 << iota
	SimplePaths
	ComplexWildcardWithPrefix
	ComplexWildcardWithSuffix

	noRules ruleState = 0
)

func (r *RootDir) generateStatemachineFor(mount string, paths []string) (group.StateMachine[int64], error) {
	rules := r.buildDirRules(mount, paths)

	return group.NewStatemachine(rules)
}

type dirTreeRule struct {
	Dir   dirState
	Rules map[string]*db.Rule
}

type RuleTree struct {
	children map[string]*RuleTree
	dirTreeRule
	HasBackup, HasChildWithBackup bool
}

func NewRuleTree() *RuleTree {
	return newRuleTree(dirTreeRule{})
}

func newRuleTree(dirTreeRule dirTreeRule) *RuleTree {
	if dirTreeRule.Rules == nil {
		dirTreeRule.Rules = map[string]*db.Rule{}
	}

	return &RuleTree{
		children:    make(map[string]*RuleTree),
		dirTreeRule: dirTreeRule,
	}
}

func (r *RuleTree) Set(path string, rules map[string]*db.Rule, changed bool) {
	curr := r

	for part := range pathParts(path[1:]) {
		next, ok := curr.children[part]
		if !ok {
			if changed && len(rules) == 0 {
				curr.Dir |= RulesChanged

				break
			}

			var new dirTreeRule

			if curr.Dir&RulesChanged != 0 || curr.Dir&ParentRulesChanged != 0 {
				new.Dir |= ParentRulesChanged
			}

			next = newRuleTree(new)
			curr.children[part] = next
		}

		curr.Dir |= HasChildWithRules

		if changed {
			curr.Dir |= HasChildWithChangedRules
		}

		curr = next
	}

	if changed {
		curr.Dir |= RulesChanged
	}

	curr.Rules = maps.Clone(rules)
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
	changed := r.Dir&RulesChanged != 0

	hasOverride := false

	for match, rule := range r.Rules {
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
	if _, ok := r.Rules[match]; ok {
		slash := strings.IndexByte(match, '/')
		wildcard := strings.IndexByte(match, '*')

		if wildcard < 0 || slash >= 0 && wildcard < slash {
			return
		}
	} else {
		r.Rules[match] = rule

		if changed {
			r.Dir |= RulesChanged
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
	changed := r.Dir&RulesChanged != 0

	for match, rule := range r.Rules {
		preWildcard, _, _ := strings.Cut(match, "*")
		slash := strings.LastIndexByte(preWildcard, '/')

		if slash < 0 {
			continue
		}

		delete(r.Rules, match)

		curr := r

		for part := range pathParts(match[:slash+1]) {
			next, ok := curr.children[part]
			if !ok {
				var new dirTreeRule

				if changed || curr.Dir&ParentRulesChanged != 0 {
					new.Dir |= ParentRulesChanged
				}

				next = newRuleTree(new)
				curr.children[part] = next
			}

			curr.Dir |= HasChildWithRules

			if changed {
				curr.Dir |= HasChildWithChangedRules
			}

			curr = next
		}

		newMatch := match[slash+1:]

		if _, ok := curr.Rules[newMatch]; !ok {
			curr.Rules[newMatch] = rule
		}
	}
}

func (r *RuleTree) MarkBackupDirs() {
	for _, rule := range r.Rules {
		if rule.BackupType == db.BackupIBackup {
			r.HasBackup = true

			break
		}
	}

	for _, child := range r.children {
		child.MarkBackupDirs()

		if child.HasBackup || child.HasChildWithBackup {
			r.HasChildWithBackup = true
		}
	}
}

func (r *RuleTree) Iter() iter.Seq2[string, *RuleTree] {
	return maps.All(r.children)
}

var basicWildcard = map[string]*db.Rule{"*": nil}

func (r *RootDir) buildDirRules(mount string, paths []string) []group.PathGroup[int64] {
	dirs := slices.Collect(maps.Keys(r.directoryRules))

	slices.Sort(dirs)

	root := NewRuleTree()
	root.Set(mount, basicWildcard, false)
	root.Set("/", basicWildcard, true)

	for _, dir := range dirs {
		if !strings.HasPrefix(dir, mount) {
			continue
		}

		root.Set(dir, r.directoryRules[dir].Rules, paths == nil || slices.Contains(paths, dir))
	}

	root.Canon()

	return buildRules(root, "/", nil, 0)
}

func buildRules(d *RuleTree, path string, rules []group.PathGroup[int64], wildcard int64) []group.PathGroup[int64] {
	if len(d.Rules) > 0 {
		if wc, ok := d.Rules["*"]; ok {
			wildcard = wc.ID()
		}

		for match, rule := range d.Rules {
			rules = addRule(rules, path+match, rule.ID())
		}
	}

	switch ruleState := getRuleState(d.Rules); ruleState {
	case noRules:
		rules = addNoRuleRules(rules, path, d.Dir, wildcard)
	case SimpleWildcard:
		rules = addSimpleWildcardRules(rules, path, d.Dir, wildcard)
	case SimplePaths:
		rules = addRule(rules, path, processRules)
	case SimpleWildcard | SimplePaths:
		rules = addSimpleRules(rules, path, d.Dir, wildcard)
	case ComplexWildcardWithPrefix, ComplexWildcardWithSuffix, ComplexWildcardWithPrefix | SimplePaths, ComplexWildcardWithSuffix | SimplePaths:
		rules = addComplexRules(rules, path, d.Dir, ruleState, wildcard, d.Rules)
	default:
		rules = addComplexWithWildcardRules(rules, path, d.Dir, ruleState, wildcard, d.Rules)
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
			rs |= SimpleWildcard
		} else if strings.Contains(match, "*") {
			if match[0] == '*' {
				rs |= ComplexWildcardWithSuffix
			} else {
				rs |= ComplexWildcardWithPrefix
			}
		} else {
			rs |= SimplePaths
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

	if dirState&RulesChanged != 0 && dirState&HasChildWithRules == 0 && dirState&ParentRulesChanged == 0 {
		process = wildcard
	}

	return addRule(rules, path, process)
}

func addSimpleWildcardRules(rules []group.PathGroup[int64], path string, dirState dirState, wildcard int64) []group.PathGroup[int64] {
	if dirState&RulesChanged != 0 {
		if dirState&HasChildWithRules != 0 {
			return addRule(addRule(rules, path, processRules), path+"*/", wildcard)
		} else {
			return addRule(rules, path, wildcard)
		}
	} else if dirState&HasChildWithChangedRules != 0 {
		return addRule(addRule(rules, path, processRules), path+"*/", -wildcard)
	}

	return addRule(rules, path, -wildcard)
}

func addSimpleRules(rules []group.PathGroup[int64], path string, dirState dirState, wildcard int64) []group.PathGroup[int64] {
	if dirState&RulesChanged != 0 {
		return addRule(addRule(rules, path, processRules), path+"*/", wildcard)
	} else if dirState&HasChildWithChangedRules != 0 {
		return addRule(addRule(rules, path, processRules), path+"*/", -wildcard)
	}

	return addRule(rules, path, -wildcard)
}

func addComplexRules(rules []group.PathGroup[int64], path string, dirState dirState, ruleState ruleState, wildcard int64, rs map[string]*db.Rule) []group.PathGroup[int64] {
	rules = addRule(rules, path, processRules)

	if dirState&RulesChanged == 0 && dirState&ParentRulesChanged == 0 {
		return addRule(rules, path+"*/", -wildcard)
	}

	return addComplexChildRules(rules, path, ruleState, wildcard, rs)
}

func addComplexChildRules(rules []group.PathGroup[int64], path string, ruleState ruleState, wildcard int64, rs map[string]*db.Rule) []group.PathGroup[int64] {
	if ruleState&ComplexWildcardWithSuffix != 0 {
		return addRule(rules, path+"*/", processRules)
	} else if ruleState&SimpleWildcard != 0 {
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
	if dirState&RulesChanged == 0 {
		if dirState&HasChildWithChangedRules != 0 {
			return addRule(addRule(rules, path, processRules), path+"*/", -wildcard)
		} else {
			return addRule(rules, path, -wildcard)
		}
	}

	rules = addRule(rules, path, processRules)

	return addComplexChildRules(rules, path, ruleState, wildcard, rs)
}
