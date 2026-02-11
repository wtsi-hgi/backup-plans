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

// Rules is a list of rule paths and IDs ready to be compiled into a
// StateMachine.
type Rules = []group.PathGroup[int64]

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

func generateStatemachineFor(mount string, paths []string,
	directoryRules map[string]*DirRules) (group.StateMachine[int64], group.StateMachine[int64], error) {
	rules, wildcards := buildDirRules(mount, paths, directoryRules)

	sm, err := group.NewStatemachine(rules)
	if err != nil {
		return nil, nil, err
	}

	wcs, err := group.NewStatemachine(wildcards)
	if err != nil {
		return nil, nil, err
	}

	return sm, wcs, nil
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

// NewRuleTree creates a rule pre-processor to enable parent 'rule overrides' and matches containing
// slashes.
//
// The RuleTree also calculates the directory level rules to determine efficient rule calculations.
// The RuleTree keeps track of which directories have changed rules, or are affected by changed
// rules.
//
// Specifically, it tracks whether a directory itself has changed, whether a parent has changed,
// whether a child directory contains rules, and whether a child directory has changed.
//
// When building the statemachine, we also read all of the rules for a directory to determine the
// type of rules it contains and how those rules affect it and child directories.
//
// These rules are put into four categories, SimplePaths (like 'abc.txt'), SimpleWildcard ('*'),
// WildcardPrefix ('*.txt'), and WildcardSuffix ('abc.*').
//
// The combination of both the directory changed status and the rule types allows us to determine
// how a particular directory needs to be processed. The following table spells out how the rules
// are determined.
//
// | Dir Status   | NR | W       | SP | W + SP  | WP or WS | WS or WS + W | WP + W  |
// |--------------|----|---------|----|---------|----------|--------------|---------|
// | NC           | -  | -       | -  | -       | -        | -            | -       |
// | NC + CP      | -  | CH      | P  | CH      | P => P   | CH           | CH      |
// | NC + C       | -  | -       | -  | -       | -        | -            | -       |
// | NC + C + CP  | -  | CH      | P  | CH      | P => P   | CH           | CH      |
// | NC + CC      | P  | P => CH | P  | P => CH | P => CH  | P => CH      | P => CH |
// | NC + CC + CP | P  | P => CH | P  | P => CH | P => P   | P => CH      | P => CH |
// | DC           | AL | AL      | P  | P => AL | P => P   | P => S       | P => AL |
// | DC + CP      | P  | AL      | P  | P => AL | P => P   | P => S       | P => AL |
// | DC + C       | P  | P => AL | P  | P => AL | P => P   | P => S       | P => AL |
// | DC + C + CP  | P  | P => AL | P  | P => AL | P => P   | P => S       | P => AL |
// | DC + CC      | P  | P => AL | P  | P => AL | P => P   | P => S       | P => AL |
// | DC + CC + CP | P  | P => AL | P  | P => AL | P => P   | P => S       | P => AL |
//
// Key:
//
//	NC = Directory rules Not Changed.
//	CP = Directory has Parent with Changed rules.
//	C  = Directory has Child with rules.
//	CC = Directory has Child with Changed rules.
//	DC = Directory has Changed rules.
//
//	NR: Directory has No Rules.
//	W : Directory has SimpleWildcard rule ('*').
//	SP: Directory has SimplePath rules ('abc.txt').
//	WP: Directory has WildcardPrefix rules ('*.txt').
//	WS: Directory has WildcardSuffix rules ('abc.*').
//
//	P : Process files in this directory as normal.
//	AL: Add the data from the lower DB (tree DB), swapping out the rule ID with
//	the wildcard ID.
//	CH: Copy from the higher DB (overlay DB) if it exists, or fall back to AL.
//	S : Special handling of WildcardSuffix rules, where we cut off after the first '*' and use that
//	to match directories that match the text before (and including) the wildcard.  If a Wildcard is
//	present, the AL rule is applied as normal.
//
// For each cell, it describes how rules are applied for that directory, and optionally how they are
// applied to unknown subdirectories (=>).
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

// Set adds the rules for the given path. The changed flag should be set true if
// this directory has had a rule changed (added, updated, or removed).
func (r *RuleTree) Set(path string, rules map[string]*db.Rule, changed bool) { //nolint:gocognit,gocyclo,funlen
	curr := r

	for part := range pathParts(path[1:]) {
		next, ok := curr.children[part]
		if !ok { //nolint:nestif
			if changed && len(rules) == 0 {
				curr.Dir |= RulesChanged

				return
			}

			var newDT dirTreeRule

			if curr.Dir&RulesChanged != 0 || curr.Dir&ParentRulesChanged != 0 {
				newDT.Dir |= ParentRulesChanged
			}

			next = newRuleTree(newDT)
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

// Canon resolves the parent rule-override and slash containing rules, producing
// a canonical rule tree.
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
	if _, ok := r.Rules[match]; ok { //nolint:nestif
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

func (r *RuleTree) resolveSlashes() { //nolint:gocognit,gocyclo,funlen
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
				var newDT dirTreeRule

				if changed || curr.Dir&ParentRulesChanged != 0 {
					newDT.Dir |= ParentRulesChanged
				}

				next = newRuleTree(newDT)
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

// MarkBackupDirs traverses the tree marking all directories that contain
// ibackup rules, and marking parents of those directories.
//
// This is to allow for efficient tree traversal, only entering sub-trees that
// contain relevant rules.
func (r *RuleTree) MarkBackupDirs() {
	r.markBackupDirs(false)
}

func (r *RuleTree) markBackupDirs(parentWithBackup bool) {
	for _, rule := range r.Rules {
		if parentWithBackup || rule.BackupType == db.BackupIBackup {
			r.HasBackup = true
			parentWithBackup = true

			break
		}
	}

	for _, child := range r.children {
		child.markBackupDirs(parentWithBackup)

		if child.HasBackup || child.HasChildWithBackup {
			r.HasChildWithBackup = true
		}
	}
}

// Iter iterates over the direct children for the current depth.
func (r *RuleTree) Iter() iter.Seq2[string, *RuleTree] {
	return maps.All(r.children)
}

var basicWildcard = map[string]*db.Rule{"*": {Match: "*"}} //nolint:gochecknoglobals

func buildDirRules(mount string, paths []string,
	directoryRules map[string]*DirRules) (Rules, Rules) {
	dirs := slices.Collect(maps.Keys(directoryRules))

	slices.Sort(dirs)

	root := NewRuleTree()
	root.Set(mount, basicWildcard, false)
	root.Set("/", basicWildcard, true)

	for _, dir := range dirs {
		if !strings.HasPrefix(dir, mount) {
			continue
		}

		root.Set(dir, directoryRules[dir].Rules, paths == nil || slices.Contains(paths, dir))
	}

	root.Canon()

	return buildRules(root, "/", nil, 0), buildWildcards(root, "/", nil)
}

func buildRules(d *RuleTree, path string, rules Rules, wildcard int64) Rules { //nolint:gocyclo,cyclop,funlen
	if len(d.Rules) > 0 {
		if wc, ok := d.Rules["*"]; ok {
			wildcard = wc.ID()
		}

		for match, rule := range d.Rules {
			rules = addRuleToList(rules, path+match, rule.ID())
		}
	}

	switch rs := getRuleState(d.Rules); rs {
	case noRules:
		rules = addNoRuleRules(rules, path, d.Dir, wildcard)
	case SimpleWildcard:
		rules = addSimpleWildcardRules(rules, path, d.Dir, wildcard)
	case SimplePaths:
		rules = addRuleToList(rules, path, processRules)
	case SimpleWildcard | SimplePaths:
		rules = addSimpleRules(rules, path, d.Dir, wildcard)
	case ComplexWildcardWithPrefix, ComplexWildcardWithSuffix,
		ComplexWildcardWithPrefix | SimplePaths, ComplexWildcardWithSuffix | SimplePaths:
		rules = addComplexRules(rules, path, d.Dir, rs, wildcard, d.Rules)
	default:
		rules = addComplexWithWildcardRules(rules, path, d.Dir, rs, wildcard, d.Rules)
	}

	for part, child := range d.children {
		rules = buildRules(child, path+part, rules, wildcard)
	}

	return rules
}

func getRuleState(rules map[string]*db.Rule) ruleState { //nolint:gocognit
	if len(rules) == 0 {
		return 0
	}

	var rs ruleState

	for match := range rules {
		if match == "*" { //nolint:gocritic,nestif
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

func addRuleToList(rules Rules, path string, rule int64) Rules {
	return append(rules, group.PathGroup[int64]{
		Path:  []byte(path),
		Group: &rule,
	})
}

func addNoRuleRules(rules Rules, path string, dirState dirState,
	wildcard int64) Rules {
	process := processRules

	if dirState&RulesChanged != 0 && dirState&HasChildWithRules == 0 && dirState&ParentRulesChanged == 0 {
		process = wildcard
	}

	return addRuleToList(rules, path, process)
}

func addSimpleWildcardRules(rules Rules, path string, dirState dirState,
	wildcard int64) Rules {
	if dirState&RulesChanged != 0 { //nolint:nestif
		if dirState&HasChildWithRules != 0 {
			return addRuleToList(addRuleToList(rules, path, processRules), path+"*/", wildcard)
		}

		return addRuleToList(rules, path, wildcard)
	} else if dirState&HasChildWithChangedRules != 0 {
		return addRuleToList(addRuleToList(rules, path, processRules), path+"*/", -wildcard)
	}

	return addRuleToList(rules, path, -wildcard)
}

func addSimpleRules(rules Rules, path string, dirState dirState,
	wildcard int64) Rules {
	if dirState&RulesChanged != 0 {
		return addRuleToList(addRuleToList(rules, path, processRules), path+"*/", wildcard)
	} else if dirState&HasChildWithChangedRules != 0 {
		return addRuleToList(addRuleToList(rules, path, processRules), path+"*/", -wildcard)
	}

	return addRuleToList(rules, path, -wildcard)
}

func addComplexRules(rules Rules, path string, dirState dirState,
	ruleState ruleState, wildcard int64, rs map[string]*db.Rule) Rules {
	rules = addRuleToList(rules, path, processRules)

	if dirState&RulesChanged == 0 && dirState&ParentRulesChanged == 0 {
		return addRuleToList(rules, path+"*/", -wildcard)
	}

	return addComplexChildRules(rules, path, ruleState, wildcard, rs)
}

func addComplexChildRules(rules Rules, path string, ruleState ruleState, //nolint:gocognit,gocyclo
	wildcard int64, rs map[string]*db.Rule) Rules {
	if ruleState&ComplexWildcardWithSuffix != 0 {
		return addRuleToList(rules, path+"*/", processRules)
	} else if ruleState&SimpleWildcard != 0 {
		rules = addRuleToList(rules, path+"*/", -wildcard)
	}

	todo := map[string]int64{}

	for match, r := range rs {
		if pos := strings.IndexByte(match, '*'); pos > 0 { //nolint:nestif
			newMatch := match[:pos]

			if _, ok := todo[newMatch]; ok || pos+1 < len(match) {
				todo[newMatch] = processRules
			} else {
				todo[newMatch] = r.ID()
			}
		}
	}

	for part, id := range todo {
		rules = addRuleToList(rules, path+part+"*/", id)
	}

	return rules
}

func addComplexWithWildcardRules(rules Rules, path string, dirState dirState,
	ruleState ruleState, wildcard int64, rs map[string]*db.Rule) Rules {
	if dirState&RulesChanged == 0 {
		if dirState&HasChildWithChangedRules != 0 {
			return addRuleToList(addRuleToList(rules, path, processRules), path+"*/", -wildcard)
		}

		return addRuleToList(rules, path, -wildcard)
	}

	rules = addRuleToList(rules, path, processRules)

	return addComplexChildRules(rules, path, ruleState, wildcard, rs)
}

func buildWildcards(d *RuleTree, path string, rules Rules) Rules { //nolint:gocyclo
	for match, rule := range d.Rules {
		id := rule.ID()

		if match == "*" && id != 0 {
			rules = append(rules,
				group.PathGroup[int64]{Path: []byte(path), Group: &id},
				group.PathGroup[int64]{Path: []byte(path + "*/"), Group: &id},
			)
		} else if rule.Override && !strings.HasPrefix(rule.Match, "*") && strings.HasSuffix(rule.Match, "/*") {
			rules = append(rules,
				group.PathGroup[int64]{Path: []byte(path + "*/" + strings.TrimSuffix(rule.Match, "*")), Group: &id},
			)
		}
	}

	for part, child := range d.children {
		rules = buildWildcards(child, path+part, rules)
	}

	return rules
}
