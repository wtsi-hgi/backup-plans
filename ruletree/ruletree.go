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
	"cmp"
	"io"
	"iter"
	"slices"
	"strings"

	"github.com/wtsi-hgi/wrstat-ui/summary/group"
	"vimagination.zapto.org/byteio"
	"vimagination.zapto.org/tree"
)

var emptyNode tree.MemTree //nolint:gochecknoglobals

// Rule contains the user and group summaries for a rule.
type Rule struct {
	ID            uint64
	Users, Groups RuleStats
}

func (r *Rule) writeTo(sw *byteio.StickyLittleEndianWriter) {
	sw.WriteUintX(r.ID)
	sw.WriteUintX(uint64(len(r.Users)))

	for n := range r.Users {
		r.Users[n].writeTo(sw)
	}

	sw.WriteUintX(uint64(len(r.Groups)))

	for n := range r.Groups {
		r.Groups[n].writeTo(sw)
	}
}

// RuleStats represents the stats for a list of users or groups.
type RuleStats []Stats

func (r *RuleStats) add(id uint32, mtime, count, size uint64) {
	newStats := Stats{
		id: id,
	}

	pos, ok := slices.BinarySearchFunc(*r, newStats, func(a, b Stats) int {
		return int(a.id) - int(b.id)
	})
	if !ok {
		*r = slices.Insert(*r, pos, newStats)
	}

	(*r)[pos].MTime = max((*r)[pos].MTime, mtime)
	(*r)[pos].Files += count
	(*r)[pos].Size += size
}

type file struct {
	uid, gid uint32
	mtime    uint64
	size     uint64
}

func (f *file) readFrom(lr byteio.MemLittleEndian) {
	f.uid = uint32(lr.ReadUintX()) //nolint:gosec
	f.gid = uint32(lr.ReadUintX()) //nolint:gosec
	f.mtime = lr.ReadUintX()
	f.size = lr.ReadUintX()
}

// ruleProcessor does the actual processing of the rules on a tree DB.
//
// Given a statemachine generated via a RuleTree, a Tree DB, and an optional
// overlay DB, will efficiently traverse the tree calculating new rule values.
//
// The starting tree DB contains, for each file, its UID, GID, size, and mtime;
// for each directory it contains its UID, GID, and a summary of *all* of the
// files contained in that directory as well as all of its descendants.
//
// The overlay DB, will contain the results of a previous use of the
// ruleProcessor. For each directory it will contain its UID, GID, and a summary
// for each of the rules affecting files within it and its descendants.
//
// The directory summary data for the tree DB and overlay DB is identical, and
// the tree DB summary is stored as rule 0, meaning unplanned. This allows for
// certain optimisations later.
//
// The summaries contain two lists, users and groups, each of which specify the
// ID of the user or group, the number of files matched, the total size of the
// files matched, and the most recent mtime of the files matched.
//
// Reading a summary from the combined tree DB and overlay DB, requires
// traversing both trees until the required directory is reached and reading the
// data from either the overlay DB (if it exists) or the tree DB.
//
// As mentioned before, given that the directory data format between the tree
// and overlay DBs is the same, one optimisation that can be done to save space
// in the overlay DB is to not store any data for a directory (and its
// descendants) if all files match a single rule, as long as we can calculate
// what the rule should be when attempting to read the summary. As such, simple
// wildcard matches ('*') will often result in no data written to the overlay
// tree and will require a simple reverse directory lookup in the stored rules
// to determine which rule ID the tree DBs 0 should be replaced with.
//
// For efficient re-calculating of rules, we only need to take into account
// directories which will be affected by changed rules. The RuleTree produces a
// set of rules that match on the directories themselves (as opposed to normal
// rules which are only applied to files). These rules fall into one of three
// categories:
//
//	ID = MinUint64: Process rules as normal, iterating through each file and
//		sub-directory,  passing each file path through the statemachine to
//		determine the rule matched against.
//
//	ID > 0: Read the tree DB summary for a sub-directory and add its summary to
//		the current directory summary, swapping out the rule number with the ID
//		on the directory.
//
//	ID <= 0: Copy the overlay DB summary for a sub-directory, if it exists, or
//		fall back to the previous category if it does not, negating the
//		directory ID to get the wildcard ID.
type ruleProcessor struct {
	lowerNode, upperNode *tree.MemTree
	sm                   group.State[int64]
	UID, GID             uint32
	Rules                []Rule
}

func (r *ruleProcessor) Children() iter.Seq2[string, tree.Node] {
	sr := byteio.MemLittleEndian(r.lowerNode.Data())

	r.UID = uint32(sr.ReadUintX()) //nolint:gosec
	r.GID = uint32(sr.ReadUintX()) //nolint:gosec

	return r.children
}

func (r *ruleProcessor) children(yield func(string, tree.Node) bool) {
	for name, child := range r.lowerNode.Children() {
		lowerChild := child.(*tree.MemTree) //nolint:errcheck,forcetypeassert

		if !strings.HasSuffix(name, "/") {
			r.processFile(name, lowerChild.Data())

			continue
		}

		upperChild, _ := r.upperNode.Child(name) //nolint:errcheck
		state := r.sm.GetStateString(name)

		var cont bool

		if ruleID := *state.GetGroup(); ruleID == processRules { //nolint:nestif
			cont = r.processDir(name, state, lowerChild, upperChild, yield)
		} else if ruleID <= 0 {
			cont = r.copyUpperOrAddLower(name, -ruleID, lowerChild, upperChild, yield)
		} else {
			cont = r.addLower(ruleID, lowerChild)
		}

		if !cont {
			break
		}
	}
}

func (r *ruleProcessor) processFile(name string, data []byte) {
	var f file

	f.readFrom(data)

	var ruleID int64

	if rule := r.sm.GetStateString(name).GetGroup(); rule != nil {
		ruleID = *rule
	}

	r.setRule(ruleID, &f)
}

func (r *ruleProcessor) setRule(ruleID int64, f *file) {
	pos := r.getRulePos(ruleID)

	r.Rules[pos].Users.add(f.uid, f.mtime, 1, f.size)
	r.Rules[pos].Groups.add(f.gid, f.mtime, 1, f.size)
}

func (r *ruleProcessor) getRulePos(ruleID int64) int {
	newRule := Rule{ID: uint64(ruleID)} //nolint:gosec

	pos, ok := slices.BinarySearchFunc(r.Rules, newRule, func(a, b Rule) int {
		return int(a.ID) - int(b.ID) //nolint:gosec
	})
	if !ok {
		r.Rules = slices.Insert(r.Rules, pos, newRule)
	}

	return pos
}

func (r *ruleProcessor) processDir(name string, state group.State[int64],
	lowerChild, upperChild *tree.MemTree, yield func(string, tree.Node) bool) bool {
	childProcessor := ruleProcessor{
		lowerNode: lowerChild,
		upperNode: cmp.Or(upperChild, &emptyNode),
		sm:        state,
	}

	if !yield(name, &childProcessor) {
		return false
	}

	r.mergeChild(&childProcessor)

	return true
}

func (r *ruleProcessor) mergeChild(child *ruleProcessor) {
	for _, rule := range child.Rules {
		pos := r.getRulePos(int64(rule.ID)) //nolint:gosec

		for _, user := range rule.Users {
			r.Rules[pos].Users.add(user.id, user.MTime, user.Files, user.Size)
		}

		for _, group := range rule.Groups {
			r.Rules[pos].Groups.add(group.id, group.MTime, group.Files, group.Size)
		}
	}
}

func (r *ruleProcessor) copyUpperOrAddLower(name string, ruleID int64,
	lowerChild, upperChild *tree.MemTree, yield func(string, tree.Node) bool) bool {
	if upperChild == nil {
		return r.addLower(ruleID, lowerChild)
	}

	sr := byteio.MemLittleEndian(upperChild.Data())

	sr.ReadUintX()
	sr.ReadUintX()

	for range sr.ReadUintX() {
		ruleID := sr.ReadUintX()

		readArray(&sr, int64(ruleID), r.addUserData)  //nolint:gosec
		readArray(&sr, int64(ruleID), r.addGroupData) //nolint:gosec
	}

	return yield(name, upperChild)
}

func readArray(sr *byteio.MemLittleEndian, ruleID int64, fn func(uint32, int64, uint64, uint64, uint64)) {
	for range sr.ReadUintX() {
		id := uint32(sr.ReadUintX()) //nolint:gosec
		mtime := sr.ReadUintX()
		files := sr.ReadUintX()
		size := sr.ReadUintX()

		fn(id, ruleID, mtime, files, size)
	}
}

func (r *ruleProcessor) addUserData(uid uint32, ruleID int64, mtime, files, size uint64) {
	pos := r.getRulePos(ruleID)

	r.Rules[pos].Users.add(uid, mtime, files, size)
}

func (r *ruleProcessor) addGroupData(gid uint32, ruleID int64, mtime, files, size uint64) {
	pos := r.getRulePos(ruleID)

	r.Rules[pos].Groups.add(gid, mtime, files, size)
}

func (r *ruleProcessor) addLower(ruleID int64, lowerChild *tree.MemTree) bool {
	sr := byteio.MemLittleEndian(lowerChild.Data())

	sr.ReadUintX()
	sr.ReadUintX()
	sr.ReadUint8()
	sr.ReadUint8()

	readArray(&sr, ruleID, r.addUserData)
	readArray(&sr, ruleID, r.addGroupData)

	return true
}

func (r *ruleProcessor) WriteTo(w io.Writer) (int64, error) {
	sw := byteio.StickyLittleEndianWriter{Writer: w}

	sw.WriteUintX(uint64(r.UID))
	sw.WriteUintX(uint64(r.GID))
	sw.WriteUintX(uint64(len(r.Rules)))

	for n := range r.Rules {
		r.Rules[n].writeTo(&sw)
	}

	return sw.Count, sw.Err
}
