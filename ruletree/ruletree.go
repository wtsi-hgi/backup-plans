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
	"sync"

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

func (r *RuleStats) add(id uint32, mtime, count, size, bcount, bsize, acount, asize uint64) {
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
	(*r)[pos].BackupFiles += bcount
	(*r)[pos].BackupSize += bsize
	(*r)[pos].ArchiveFiles += acount
	(*r)[pos].ArchiveSize += asize
}

type treeFile struct {
	HasFile, HasBackup, HasArchive uint64
	UID, GID                       uint32
	MTime                          uint64
	Size                           uint64
	BackupSize                     uint64
	ArchiveSize                    uint64
}

func (t *treeFile) readFrom(lr byteio.MemLittleEndian) {
	if len(lr) == 0 {
		return
	}

	t.HasFile = 1
	t.UID = uint32(lr.ReadUintX()) //nolint:gosec
	t.GID = uint32(lr.ReadUintX()) //nolint:gosec
	t.MTime = lr.ReadUintX()
	t.Size = lr.ReadUintX()
}

func (t *treeFile) readBackupDataFrom(lr byteio.MemLittleEndian) {
	if len(lr) == 0 {
		return
	}

	size := lr.ReadUintX()

	if lr.ReadBool() {
		t.HasArchive = 1
		t.ArchiveSize = size
	} else {
		t.HasBackup = 1
		t.BackupSize = size
	}
}

type treeNode struct {
	lowerNode, upperNode, backups *tree.MemTree
}

func (t *treeNode) Owner() (uint32, uint32) {
	sr := byteio.MemLittleEndian(t.lowerNode.Data())

	return uint32(sr.ReadUintX()), uint32(sr.ReadUintX()) //nolint:gosec
}

func (t *treeNode) Children() iter.Seq2[string, treeNode] {
	return func(yield func(string, treeNode) bool) {
		nextChild, childStop := iter.Pull2(t.lowerNode.Children())
		nextBackup, backupStop := iter.Pull2(t.backups.Children())

		defer backupStop()
		defer childStop()

		var (
			childName, backupName        string
			childNode, backupNode, upper *tree.MemTree
		)

		childOK, backupOK := true, true
		updateChild, updateBackup := true, true

		for childOK || backupOK {
			if updateChild {
				updateChild = false
				childName, childNode, childOK = nextNode(nextChild)

				if childOK {
					u, _ := t.upperNode.Child(childName)
					upper = cmp.Or(u, &emptyNode)
				}
			}

			if updateBackup {
				updateBackup = false
				backupName, backupNode, backupOK = nextNode(nextBackup)

				if backupName == "" && backupOK {
					backupName, backupNode, backupOK = nextNode(nextBackup)
				}
			}

			if !childOK && !backupOK {
				break
			}

			var (
				tn   = treeNode{&emptyNode, &emptyNode, &emptyNode}
				name string
			)

			n := strings.Compare(childName, backupName)

			if childOK && (!backupOK || n != 1) {
				name = childName
				tn.lowerNode = childNode
				tn.upperNode = upper
				updateChild = true
			}

			if backupOK && (!childOK || n != -1) {
				name = backupName
				tn.backups = backupNode
				updateBackup = true
			}

			if !yield(name, tn) {
				break
			}
		}
	}
}

func nextNode(next func() (string, tree.Node, bool)) (string, *tree.MemTree, bool) {
	name, node, ok := next()

	if ok {
		return name, node.(*tree.MemTree), ok
	}

	return "", nil, false
}

func (t *treeNode) HasUpper() bool {
	return t.upperNode != &emptyNode
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
//	ID >= 0: Read the tree DB summary for a sub-directory and add its summary to
//		the current directory summary, swapping out the rule number with the ID
//		on the directory.
//
//	ID < 0: Copy the overlay DB summary for a sub-directory, if it exists, or
//		fall back to the previous category if it does not, negating the
//		directory ID, and subtracting 1, to get the wildcard ID.
type ruleProcessor struct {
	UID, GID uint32
	Rules    []Rule
	children []namedNode
}

func (r *ruleProcessor) process(node treeNode, sm State, pwg *sync.WaitGroup) {
	defer pwg.Done()

	var wg sync.WaitGroup

	r.UID, r.GID = node.Owner()

	for name, child := range node.Children() {
		if !strings.HasSuffix(name, "/") {
			r.processFile(sm, name, child)

			continue
		}

		state := sm.GetStateString(name)

		if ruleID := *state.GetGroup(); ruleID == processRules { //nolint:nestif
			r.processDir(name, state, child, &wg)
		} else if ruleID < 0 {
			r.copyUpperOrAddLower(name, -ruleID-1, child)
		} else {
			r.addLower(ruleID, child)
		}
	}

	r.waitForChildren(&wg)
}

func (r *ruleProcessor) waitForChildren(wg *sync.WaitGroup) {
	wg.Wait()

	for _, child := range r.children {
		if c, ok := child.Node.(*ruleProcessor); ok {
			r.mergeChild(c)
		}
	}
}

func (r *ruleProcessor) processFile(sm State, name string, file treeNode) {
	var t treeFile

	t.readFrom(file.lowerNode.Data())
	t.readBackupDataFrom(file.backups.Data())

	var ruleID int64

	if rule := sm.GetStateString(name).GetGroup(); rule != nil {
		ruleID = *rule
	}

	r.setRule(ruleID, &t)
}

func (r *ruleProcessor) setRule(ruleID int64, f *treeFile) {
	pos := r.getRulePos(ruleID)

	r.Rules[pos].Users.add(f.UID, f.MTime, f.HasFile, f.Size, f.HasBackup, f.BackupSize, f.HasArchive, f.ArchiveSize)
	r.Rules[pos].Groups.add(f.GID, f.MTime, f.HasFile, f.Size, f.HasBackup, f.BackupSize, f.HasArchive, f.ArchiveSize)
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

func (r *ruleProcessor) processDir(name string, state State, child treeNode, wg *sync.WaitGroup) {
	c := &ruleProcessor{}

	r.children = append(r.children, namedNode{name: name, Node: c})

	wg.Add(1)

	go c.process(child, state, wg)
}

func (r *ruleProcessor) mergeChild(child *ruleProcessor) {
	for _, rule := range child.Rules {
		pos := r.getRulePos(int64(rule.ID)) //nolint:gosec

		for _, user := range rule.Users {
			r.Rules[pos].Users.add(user.id, user.MTime, user.Files, user.Size, user.BackupFiles, user.BackupSize, user.ArchiveFiles, user.ArchiveSize)
		}

		for _, group := range rule.Groups {
			r.Rules[pos].Groups.add(group.id, group.MTime, group.Files, group.Size, group.BackupFiles, group.BackupSize, group.ArchiveFiles, group.ArchiveSize)
		}
	}
}

func (r *ruleProcessor) copyUpperOrAddLower(name string, wildcard int64, child treeNode) {
	if !child.HasUpper() {
		r.addLower(wildcard, child)

		return
	}

	sr := byteio.MemLittleEndian(child.upperNode.Data())

	sr.ReadUintX()
	sr.ReadUintX()

	for range sr.ReadUintX() {
		ruleID := sr.ReadUintX()

		readArray(&sr, int64(ruleID), r.addUserData)  //nolint:gosec
		readArray(&sr, int64(ruleID), r.addGroupData) //nolint:gosec
	}

	r.children = append(r.children, namedNode{name: name, Node: child.upperNode})
}

func readArray(sr *byteio.MemLittleEndian, ruleID int64, fn func(uint32, int64, uint64, uint64, uint64, uint64, uint64, uint64, uint64)) {
	for range sr.ReadUintX() {
		id := uint32(sr.ReadUintX()) //nolint:gosec
		mtime := sr.ReadUintX()
		files := sr.ReadUintX()
		size := sr.ReadUintX()
		backupFiles := sr.ReadUintX()
		backupSize := sr.ReadUintX()
		archiveSize := sr.ReadUintX()
		archiveCount := sr.ReadUintX()

		fn(id, ruleID, mtime, files, size, backupFiles, backupSize, archiveCount, archiveSize)
	}
}

func (r *ruleProcessor) addUserData(uid uint32, ruleID int64, mtime, files, size, bFiles, bSize, aFiles, aSize uint64) {
	pos := r.getRulePos(ruleID)

	r.Rules[pos].Users.add(uid, mtime, files, size, bFiles, bSize, aFiles, aSize)
}

func (r *ruleProcessor) addGroupData(gid uint32, ruleID int64, mtime, files, size, bFiles, bSize, aFiles, aSize uint64) {
	pos := r.getRulePos(ruleID)

	r.Rules[pos].Groups.add(gid, mtime, files, size, bFiles, bSize, aFiles, aSize)
}

func (r *ruleProcessor) addLower(ruleID int64, child treeNode) {
	sr := byteio.MemLittleEndian(child.lowerNode.Data())

	uid := uint32(sr.ReadUintX())
	gid := uint32(sr.ReadUintX())
	sr.ReadUint8()
	sr.ReadUint8()

	readArray(&sr, ruleID, r.addUserData)
	readArray(&sr, ruleID, r.addGroupData)

	sr = child.backups.Data()

	if len(sr) == 0 {
		return
	}

	backupSize := sr.ReadUintX()
	backupCount := sr.ReadUintX()
	archiveSize := sr.ReadUintX()
	archiveCount := sr.ReadUintX()

	r.addUserData(uid, ruleID, 0, 0, 0, backupCount, backupSize, archiveCount, archiveSize)
	r.addGroupData(gid, ruleID, 0, 0, 0, backupCount, backupSize, archiveCount, archiveSize)
}

func (r *ruleProcessor) WriteTo(w io.Writer) (int64, error) {
	sw := w.(*byteio.StickyLittleEndianWriter) //nolint:errcheck,forcetypeassert

	sw.WriteUintX(uint64(r.UID))
	sw.WriteUintX(uint64(r.GID))
	sw.WriteUintX(uint64(len(r.Rules)))

	for n := range r.Rules {
		r.Rules[n].writeTo(sw)
	}

	return sw.Count, sw.Err
}

func (r *ruleProcessor) Children() iter.Seq2[string, tree.Node] {
	return func(yield func(string, tree.Node) bool) {
		for n := range r.children {
			if !yield(r.children[n].name, r.children[n]) {
				return
			}
		}
	}
}

type namedNode struct {
	name string
	tree.Node
}
