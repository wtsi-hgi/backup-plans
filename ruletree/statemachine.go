/*******************************************************************************
 * Copyright (c) 2025 Genome Research Ltd.
 *
 * Author: Michael Woolnough <mw31@sanger.ac.uk>
 *
 * Permission is hereby granted, free of charge, to any person obtaining
 * a copy of this software and associated documentation files (the
 * "Software"), to deal in the Software without restriction, including
 * without limitation the rights to use, copy, modify, merge, publish,
 * distribute, sublicense, and/or sell copies of the Software, and to
 * permit persons to whom the Software is furnished to do so, subject to
 * the following conditions:
 *
 * The above copyright notice and this permission notice shall be included
 * in all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
 * EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
 * MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
 * IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
 * CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
 * TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
 * SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 ******************************************************************************/

package ruletree

import (
	"bytes"
	"iter"
	"maps"
	"math"
	"slices"
	"strings"
	"unsafe"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/rules"
	"github.com/wtsi-hgi/wrstat-ui/summary/group"
)

const (
	processRules          int64 = math.MinInt64
	maxWildcardsForSimple       = 2
)

var noMatchingRules = new(int64) //nolint:gochecknoglobals

// State represents a partial match that can be continued or completed.
type State struct {
	groups []group.State[int64]
}

// BuildMultiStateMachine take a slice of rulesets and builds a statemachine for
// each.
//
// Returned State handles like group.State, but iterates over each individual
// StateMachine, removing those that no longer apply.
//
// The slice should be ordered by precedence, with the highest first.
func BuildMultiStateMachine(rules []Rules) (State, error) {
	groups := make([]group.State[int64], len(rules))

	for n, rs := range rules {
		sm, err := group.NewStatemachine(rs)
		if err != nil {
			return State{}, err
		}

		setDefaultGroup(sm)

		groups[n] = sm.GetState(nil)
	}

	return State{groups}, nil
}

func setDefaultGroup(sm group.StateMachine[int64]) {
	(*struct {
		_     [256]int32
		Group *int64
	})(unsafe.Pointer(&sm[0])).Group = noMatchingRules
}

// GetState continues matching with the given match string, returning a new
// State.
func (s State) GetStateString(match string) State {
	gs := make([]group.State[int64], 0, len(s.groups))

	for _, g := range s.groups {
		if rs := g.GetStateString(match); rs.GetGroup() != noMatchingRules {
			gs = append(gs, rs)
		}
	}

	return State{groups: gs}
}

// GetGroup returns the group at the current state.
//
// When multiple statemachines exist, the first valid group is returned.
func (s State) GetGroup() *int64 {
	for _, g := range s.groups {
		if r := g.GetGroup(); r != noMatchingRules && r != nil {
			return r
		}
	}

	return nil
}

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
	directoryRules *rules.Database) (State, group.StateMachine[int64], error) {
	rules, wildcards := buildDirRules(mount, paths, directoryRules)

	sm, err := BuildMultiStateMachine(rules)
	if err != nil {
		return State{}, nil, err
	}

	wcs, err := group.NewStatemachine(wildcards)
	if err != nil {
		return State{}, nil, err
	}

	return sm, wcs, nil
}

type dirTreeRule struct {
	Dir   dirState
	Rules map[string]rules.Rule
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
		dirTreeRule.Rules = map[string]rules.Rule{}
	}

	return &RuleTree{
		children:    make(map[string]*RuleTree),
		dirTreeRule: dirTreeRule,
	}
}

// Set adds the rules for the given path. The changed flag should be set true if
// this directory has had a rule changed (added, updated, or removed).
func (r *RuleTree) Set(path string, ruleList []rules.Rule, changed bool) { //nolint:gocognit,gocyclo,funlen
	curr := r

	for part := range pathParts(path[1:]) {
		next, ok := curr.children[part]
		if !ok { //nolint:nestif
			if changed && len(ruleList) == 0 {
				curr.Dir |= RulesChanged
			}

			var newDT dirTreeRule

			if curr.Dir&RulesChanged != 0 || curr.Dir&ParentRulesChanged != 0 {
				newDT.Dir |= ParentRulesChanged
			}

			next = newRuleTree(newDT)
			curr.children[part] = next
		}

		if len(ruleList) > 0 {
			curr.Dir |= HasChildWithRules

			if changed {
				curr.Dir |= HasChildWithChangedRules
			}
		}

		curr = next
	}

	if changed {
		curr.Dir |= RulesChanged
	}

	curr.Rules = rulesToMap(ruleList)
}

func rulesToMap(ruleList []rules.Rule) map[string]rules.Rule {
	m := make(map[string]rules.Rule, len(ruleList))

	for _, r := range ruleList {
		m[r.Match] = r
	}

	return m
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
		if rule.ID == 0 || !rule.Override {
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

func (r *RuleTree) setOverride(match string, rule rules.Rule, changed bool) {
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

var star = []byte{'*'} //nolint:gochecknoglobals

// BuildRules generates a slice of Rule sets from the current RuleTree.
//
// The sets are split based on a scoring mechanism that intends to stop the
// StateMachine sizes becoming unwieldy, but while not significantly increasing
// the runtime of the rule engine.
//
// The returned slice of Rules should be given to BuildMultiStateMachine to
// build the combined statemachine.
func (r *RuleTree) BuildRules() []Rules {
	const maxScore = 1 << 24

	rules, _ := r.buildRules("/", []Rules{nil}, 1)
	prules := []Rules{rules[0]}
	score := maxScore + 1

	for _, ruleList := range slices.Backward(rules[1:]) {
		for _, rule := range ruleList {
			starScore := factorial(bytes.Count(rule.Path, star))
			if nScore := score * starScore; nScore > maxScore {
				prules = append(prules, nil)
				score = starScore
			} else {
				score = nScore
			}

			prules[len(prules)-1] = append(prules[len(prules)-1], rule)
		}
	}

	return prules
}

func factorial(n int) int {
	const maxFactorial = 10

	if n > maxFactorial {
		return math.MaxInt
	}

	r := 1

	for i := range n {
		r *= (i + 1)
	}

	return r
}

func (r *RuleTree) buildRules(path string, rules []Rules, depth int) ([]Rules, bool) {
	var childHasVeryComplex bool

	rules, childHasVeryComplex = r.buildChildRules(path, rules, depth+1)
	hasVeryComplex := childHasVeryComplex || r.hasComplexRules()
	rules = r.processRules(path, rules, hasVeryComplex, depth)

	return rules, hasVeryComplex
}

func (r *RuleTree) buildChildRules(path string, rules []Rules, depth int) ([]Rules, bool) {
	var childHasVeryComplex bool

	for part, child := range r.children {
		var complexChild bool

		rules, complexChild = child.buildRules(path+part, rules, depth)
		if complexChild {
			childHasVeryComplex = true
		}
	}

	return rules, childHasVeryComplex
}

func (r *RuleTree) hasComplexRules() bool {
	for match := range r.Rules {
		if strings.Count(match, "*") > maxWildcardsForSimple {
			return true
		}
	}

	return false
}

func (r *RuleTree) processRules(path string, rules []Rules, hasVeryComplex bool, depth int) []Rules {
	var idx int

	if hasVeryComplex {
		idx = depth

		if len(rules) <= idx {
			rules = slices.Grow(rules, idx-len(rules)+1)[:idx+1]
		}
	}

	for match, rule := range orderRulesByPrecedence(r.Rules) {
		rules[idx] = addRuleToList(rules[idx], path+match, rule.ID)
	}

	return rules
}

func orderRulesByPrecedence(rs map[string]rules.Rule) iter.Seq2[string, rules.Rule] {
	return func(yield func(string, rules.Rule) bool) {
		keys := slices.Collect(maps.Keys(rs))

		slices.SortFunc(keys, matchPrecedence)

		for _, key := range keys {
			if !yield(key, rs[key]) {
				return
			}
		}
	}
}

func matchPrecedence(a, b string) int { //nolint:gocognit,gocyclo
	if len(a) == 0 { //nolint:nestif
		if len(b) == 0 {
			return 0
		}

		return 1
	} else if len(b) == 0 {
		return -1
	}

	aPos := strings.IndexByte(a, '*')
	bPos := strings.IndexByte(b, '*')

	if aPos == -1 { //nolint:gocritic,nestif
		if bPos == -1 {
			return len(b) - len(a)
		}

		return -1
	} else if bPos == -1 {
		return 1
	} else if a[:aPos] == b[:bPos] {
		return matchPrecedence(a[aPos+1:], b[bPos+1:])
	}

	return bPos - aPos
}

func (r *RuleTree) addRuleStates(path string, rules Rules, wildcard int64) Rules { //nolint:gocyclo
	if wc, ok := r.Rules["*"]; ok {
		wildcard = wc.ID
	}

	for part, child := range r.children {
		rules = child.addRuleStates(path+part, rules, wildcard)
	}

	switch rs := getRuleState(r.Rules); rs {
	case noRules:
		rules = addNoRuleRules(rules, path, r.Dir, wildcard)
	case SimpleWildcard:
		rules = addSimpleWildcardRules(rules, path, r.Dir, wildcard)
	case SimplePaths:
		rules = addRuleToList(rules, path, processRules)
	case SimpleWildcard | SimplePaths:
		rules = addSimpleRules(rules, path, r.Dir, wildcard)
	case ComplexWildcardWithPrefix, ComplexWildcardWithSuffix,
		ComplexWildcardWithPrefix | SimplePaths, ComplexWildcardWithSuffix | SimplePaths:
		rules = addComplexRules(rules, path, r.Dir, rs, wildcard, r.Rules)
	default:
		rules = addComplexWithWildcardRules(rules, path, r.Dir, rs, wildcard, r.Rules)
	}

	return rules
}

var basicWildcard = []rules.Rule{{Match: "*"}} //nolint:gochecknoglobals

func buildDirRules(mount string, paths []string,
	directoryRules *rules.Database) ([]Rules, Rules) {
	dirs := slices.Collect(directoryRules.Dirs())

	slices.SortFunc(dirs, func(a, b rules.Directory) int {
		return strings.Compare(a.Path, b.Path)
	})

	root := NewRuleTree()
	root.Set(mount, basicWildcard, false)
	root.Set("/", basicWildcard, true)

	for _, dir := range dirs {
		if !strings.HasPrefix(dir.Path, mount) {
			continue
		}

		root.Set(dir.Path, slices.Collect(directoryRules.DirRules(dir.Path)), paths == nil || slices.Contains(paths, dir.Path))
	}

	root.Canon()

	rules := root.BuildRules()
	rules[0] = root.addRuleStates("/", rules[0], 0)

	return rules, buildWildcards(root, "/", nil)
}

func getRuleState(rules map[string]rules.Rule) ruleState { //nolint:gocognit
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

func addNoRuleRules(rules Rules, path string, dirState dirState, wildcard int64) Rules {
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
		return addRuleToList(addRuleToList(rules, path, processRules), path+"*/", -wildcard-1)
	}

	return addRuleToList(rules, path, -wildcard-1)
}

func addSimpleRules(rules Rules, path string, dirState dirState,
	wildcard int64) Rules {
	if dirState&RulesChanged != 0 {
		return addRuleToList(addRuleToList(rules, path, processRules), path+"*/", wildcard)
	} else if dirState&HasChildWithChangedRules != 0 {
		return addRuleToList(addRuleToList(rules, path, processRules), path+"*/", -wildcard-1)
	}

	return addRuleToList(rules, path, -wildcard-1)
}

func addComplexRules(rules Rules, path string, dirState dirState,
	ruleState ruleState, wildcard int64, rs map[string]rules.Rule) Rules {
	rules = addRuleToList(rules, path, processRules)

	if dirState&RulesChanged == 0 && dirState&ParentRulesChanged == 0 {
		return addRuleToList(rules, path+"*/", -wildcard-1)
	}

	return addComplexChildRules(rules, path, ruleState, wildcard, rs)
}

func addComplexChildRules(rules Rules, path string, ruleState ruleState, //nolint:gocognit,gocyclo
	wildcard int64, rs map[string]rules.Rule) Rules {
	if ruleState&ComplexWildcardWithSuffix != 0 {
		return addRuleToList(rules, path+"*/", processRules)
	} else if ruleState&SimpleWildcard != 0 {
		rules = addRuleToList(rules, path+"*/", -wildcard-1)
	}

	todo := map[string]int64{}

	for match, r := range rs {
		if pos := strings.IndexByte(match, '*'); pos > 0 { //nolint:nestif
			newMatch := match[:pos]

			if _, ok := todo[newMatch]; ok || pos+1 < len(match) {
				todo[newMatch] = processRules
			} else {
				todo[newMatch] = r.ID
			}
		}
	}

	for part, id := range todo {
		rules = addRuleToList(rules, path+part+"*/", id)
	}

	return rules
}

func addComplexWithWildcardRules(rules Rules, path string, dirState dirState,
	ruleState ruleState, wildcard int64, rs map[string]rules.Rule) Rules {
	if dirState&RulesChanged == 0 {
		if dirState&HasChildWithChangedRules != 0 {
			return addRuleToList(addRuleToList(rules, path, processRules), path+"*/", -wildcard-1)
		}

		return addRuleToList(rules, path, -wildcard-1)
	}

	rules = addRuleToList(rules, path, processRules)

	return addComplexChildRules(rules, path, ruleState, wildcard, rs)
}

func buildWildcards(d *RuleTree, path string, rules Rules) Rules { //nolint:gocyclo
	for match, rule := range d.Rules {
		id := rule.ID

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
