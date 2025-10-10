package ruletree

import (
	"bytes"
	"io"
	"iter"
	"slices"
	"strings"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/wrstat-ui/summary/group"
	"vimagination.zapto.org/byteio"
	"vimagination.zapto.org/tree"
)

type Rule struct {
	ID            uint64
	Users, Groups ruleStats
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

type ruleStats []Stats

func (r *ruleStats) add(id uint32, mtime, count, size uint64) {
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

type rulesDir struct {
	node *tree.MemTree
	sm   group.State[db.Rule]

	uid, gid uint32

	rules []Rule
}

func (r *rulesDir) initIDs() {
	sr := byteio.StickyLittleEndianReader{Reader: bytes.NewReader(r.node.Data())}

	r.uid = uint32(sr.ReadUintX())
	r.gid = uint32(sr.ReadUintX())

	r.rules = r.rules[:0]
}

type RulesDir struct {
	rulesDir

	child *RulesDir
}

func (r *RulesDir) Children() iter.Seq2[string, tree.Node] {
	r.initIDs()
	r.initChildren()

	return r.children
}

func (r *RulesDir) initChildren() {
	if r.child == nil {
		r.child = new(RulesDir)
	}
}

func (r *RulesDir) children(yield func(string, tree.Node) bool) {
	for name, child := range r.node.Children() {
		mchild := child.(*tree.MemTree)

		if strings.HasSuffix(name, "/") {
			r.child.sm = r.sm.GetStateString(name)
			r.child.node = mchild

			if !yield(name, r.child) {
				return
			}

			r.mergeChild(&r.child.rulesDir)
		} else if err := r.processFile(name, mchild.Data()); err != nil {
			yield(name, tree.NewChildrenError(err))

			return
		}
	}
}

func (r *rulesDir) processFile(name string, data []byte) error {
	var f file

	if _, err := f.ReadFrom(bytes.NewReader(data)); err != nil {
		return err
	}

	rule := r.sm.GetStateString(name).GetGroup()

	r.setRule(rule, &f)

	return nil
}

func (r *rulesDir) mergeChild(child *rulesDir) {
	for _, rule := range child.rules {
		pos := r.getRulePos(int64(rule.ID))

		for _, user := range rule.Users {
			r.rules[pos].Users.add(user.id, user.MTime, user.Files, user.Size)
		}

		for _, group := range rule.Groups {
			r.rules[pos].Groups.add(group.id, group.MTime, group.Files, group.Size)
		}
	}
}

func (r *rulesDir) WriteTo(w io.Writer) (int64, error) {
	sw := byteio.StickyLittleEndianWriter{Writer: w}

	sw.WriteUintX(uint64(r.uid))
	sw.WriteUintX(uint64(r.gid))
	sw.WriteUintX(uint64(len(r.rules)))

	for n := range r.rules {
		r.rules[n].writeTo(&sw)
	}

	return sw.Count, sw.Err
}

func (r *rulesDir) setRule(rule *db.Rule, f *file) {
	pos := r.getRulePos(rule.ID())

	r.rules[pos].Users.add(f.uid, f.mtime, 1, f.size)
	r.rules[pos].Groups.add(f.gid, f.mtime, 1, f.size)
}

func (r *rulesDir) getRulePos(ruleID int64) int {
	newRule := Rule{ID: uint64(ruleID)}

	pos, ok := slices.BinarySearchFunc(r.rules, newRule, func(a, b Rule) int {
		return int(a.ID - b.ID)
	})
	if !ok {
		r.rules = slices.Insert(r.rules, pos, newRule)
	}

	return pos
}

func (r *rulesDir) addUserData(uid uint32, ruleID int64, mtime, files, size uint64) {
	pos := r.getRulePos(ruleID)

	r.rules[pos].Users.add(uid, mtime, files, size)
}

func (r *rulesDir) addGroupData(gid uint32, ruleID int64, mtime, files, size uint64) {
	pos := r.getRulePos(ruleID)

	r.rules[pos].Groups.add(gid, mtime, files, size)
}

func (r *rulesDir) addExisting(data []byte) {
	sr := byteio.StickyLittleEndianReader{Reader: bytes.NewReader(data)}

	sr.ReadUintX()
	sr.ReadUintX()

	for range sr.ReadUintX() {
		ruleID := sr.ReadUintX()

		readArray(&sr, int64(ruleID), r.addUserData)
		readArray(&sr, int64(ruleID), r.addGroupData)
	}
}

func readArray(sr *byteio.StickyLittleEndianReader, ruleID int64, fn func(uint32, int64, uint64, uint64, uint64)) {
	for range sr.ReadUintX() {
		uid := uint32(sr.ReadUintX())
		mtime := sr.ReadUintX()
		files := sr.ReadUintX()
		size := sr.ReadUintX()

		fn(uid, ruleID, mtime, files, size)
	}
}

type file struct {
	uid, gid uint32
	mtime    uint64
	size     uint64
}

func (f *file) ReadFrom(r io.Reader) (int64, error) {
	lr := byteio.StickyLittleEndianReader{Reader: r}

	f.uid = uint32(lr.ReadUintX())
	f.gid = uint32(lr.ReadUintX())
	f.mtime = lr.ReadUintX()
	f.size = lr.ReadUintX()

	return lr.Count, lr.Err
}

type RuleLessDir struct {
	rulesDir
	ruleDirPrefixes map[string]bool
	nameBuf         []byte

	child *RuleLessDir
	rules *RulesDir
}

func (r *RuleLessDir) Children() iter.Seq2[string, tree.Node] {
	r.initIDs()
	r.initChildren()

	return r.children
}

func (r *RuleLessDir) initChildren() {
	if r.child == nil {
		r.child = &RuleLessDir{
			rules:           r.rules,
			ruleDirPrefixes: r.ruleDirPrefixes,
		}
	}

	r.rulesDir.rules = r.rulesDir.rules[:0]
}

func (r *RuleLessDir) children(yield func(string, tree.Node) bool) {
	for name, child := range r.node.Children() {
		mchild := child.(*tree.MemTree)

		if !strings.HasSuffix(name, "/") {
			if err := r.processFile(name, mchild.Data()); err != nil {
				yield(name, tree.NewChildrenError(err))

				return
			}

			continue
		}

		nameBuf := append(r.nameBuf, name...)

		hasRules, isPrefix := r.ruleDirPrefixes[string(nameBuf)]
		if !isPrefix {
			r.addExisting(mchild.Data())

			continue
		}

		if !hasRules {
			r.child.node = mchild
			r.child.nameBuf = nameBuf
			r.child.sm = r.sm.GetStateString(name)

			if !yield(name, r.child) {
				return
			}

			r.mergeChild(&r.child.rulesDir)

			continue
		}

		r.rules.node = mchild
		r.rules.sm = r.sm.GetStateString(name)

		if !yield(name, r.rules) {
			return
		}

		r.mergeChild(&r.rules.rulesDir)
	}
}

type RuleLessDirPatch struct {
	rulesDir
	ruleDirPrefixes map[string]bool
	previousRules   *tree.MemTree
	nameBuf         []byte
}

func (r *RuleLessDirPatch) Children() iter.Seq2[string, tree.Node] {
	r.initIDs()

	return r.children
}

func (r *RuleLessDirPatch) children(yield func(string, tree.Node) bool) {
	for name, child := range r.node.Children() {
		mchild := child.(*tree.MemTree)

		if !strings.HasSuffix(name, "/") {
			if err := r.processFile(name, mchild.Data()); err != nil {
				yield(name, tree.NewChildrenError(err))

				return
			}

			continue
		}

		pchild, _ := r.previousRules.Child(name)
		nameBuf := append(r.nameBuf, name...)

		hasRules, isPrefix := r.ruleDirPrefixes[string(nameBuf)]
		if !isPrefix {
			if pchild != nil {
				r.addExisting(pchild.Data())

				if !yield(name, pchild) {
					return
				}
			} else {
				r.addExisting(mchild.Data())
			}

			continue
		}

		rd := rulesDir{
			node: mchild,
			sm:   r.sm.GetStateString(name),
		}

		if !hasRules {
			var rchild tree.Node

			var child *rulesDir

			if pchild == nil {
				childr := &RuleLessDir{
					rulesDir:        rd,
					ruleDirPrefixes: r.ruleDirPrefixes,
					nameBuf:         nameBuf,
					rules:           new(RulesDir),
				}
				child = &childr.rulesDir
				rchild = childr
			} else {
				childr := &RuleLessDirPatch{
					rulesDir:        rd,
					ruleDirPrefixes: r.ruleDirPrefixes,
					previousRules:   pchild,
					nameBuf:         nameBuf,
				}
				child = &childr.rulesDir
				rchild = childr
			}

			if !yield(name, rchild) {
				return
			}

			r.mergeChild(child)

			continue
		}

		rules := &RulesDir{
			rulesDir: rd,
		}

		if !yield(name, rules) {
			return
		}

		r.mergeChild(&rules.rulesDir)
	}
}
