package ruletree

import (
	"bytes"
	"cmp"
	"io"
	"iter"
	"slices"
	"strings"
	"unsafe"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/wrstat-ui/summary"
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
	sm   group.StateMachine[db.Rule]
	dir  summary.DirectoryPath

	uid, gid uint32

	rules []Rule

	parent interface {
		setRule(rule *db.Rule, f *file)
		addUserData(uid uint32, ruleID int64, mtime, files, bytes uint64)
		addGroupData(gid uint32, ruleID int64, mtime, files, bytes uint64)
	}
}

func (r *rulesDir) initIDs() {
	sr := byteio.StickyLittleEndianReader{Reader: bytes.NewReader(r.node.Data())}

	r.uid = uint32(sr.ReadUintX())
	r.gid = uint32(sr.ReadUintX())
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
		r.child = &RulesDir{
			rulesDir: rulesDir{
				sm: r.sm,
			},
		}
	}

	r.child.parent = r
	r.child.dir.Parent = &r.dir
	r.rules = r.rules[:0]
}

func (r *RulesDir) children(yield func(string, tree.Node) bool) {
	for name, child := range r.node.Children() {
		mchild := child.(*tree.MemTree)

		if strings.HasSuffix(name, "/") {
			r.child.dir.Name = name
			r.child.node = mchild

			if !yield(name, r.child) {
				return
			}
		} else if err := r.processFile(name, mchild.Data()); err != nil {
			yield(name, tree.NewChildrenError(err))

			return
		}
	}
}

func (r *rulesDir) processFile(name string, data []byte) error {
	var f file

	rule := r.sm.GetGroup(&summary.FileInfo{Path: &r.dir, Name: fileName(name)})

	if _, err := f.ReadFrom(bytes.NewReader(data)); err != nil {
		return err
	}

	r.setRule(rule, &f)

	return nil
}

func fileName(str string) []byte {
	return unsafe.Slice(unsafe.StringData(str), len(str))
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
	if r.parent != nil {
		r.parent.setRule(rule, f)
	}

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

func (r *rulesDir) addUserData(uid uint32, ruleID int64, mtime, files, bytes uint64) {
	if r.parent != nil {
		r.parent.addUserData(uid, ruleID, mtime, files, bytes)
	}

	pos := r.getRulePos(ruleID)

	r.rules[pos].Users.add(uid, mtime, files, bytes)
}

func (r *rulesDir) addGroupData(gid uint32, ruleID int64, mtime, files, bytes uint64) {
	if r.parent != nil {
		r.parent.addGroupData(gid, ruleID, mtime, files, bytes)
	}

	pos := r.getRulePos(ruleID)

	r.rules[pos].Groups.add(gid, mtime, files, bytes)
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
		bytes := sr.ReadUintX()

		fn(uid, ruleID, mtime, files, bytes)
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
	nameBuf         *[4096]byte

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
			rulesDir: rulesDir{
				sm: r.sm,
			},
			rules:           r.rules,
			ruleDirPrefixes: r.ruleDirPrefixes,
			nameBuf:         r.nameBuf,
		}
	}

	r.child.parent = r
	r.child.dir.Parent = &r.dir
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

		r.child.dir.Name = name

		hasRules, isPrefix := r.ruleDirPrefixes[string(r.child.dir.AppendTo(r.nameBuf[:0]))]
		if !isPrefix {
			r.addExisting(mchild.Data())

			continue
		}

		if !hasRules {
			r.child.node = mchild
			r.child.parent = r

			if !yield(name, r.child) {
				return
			}

			continue
		}

		r.rules.node = mchild
		r.rules.dir = r.child.dir
		r.rules.parent = r

		if !yield(name, r.rules) {
			return
		}
	}
}

type RuleLessDirPatch struct {
	rulesDir
	ruleDirPrefixes map[string]bool
	previousRules   *tree.MemTree
	nameBuf         *[4096]byte
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

		dir := summary.DirectoryPath{
			Parent: &r.dir,
			Name:   name,
		}

		pchild, _ := r.previousRules.Child(name)

		hasRules, isPrefix := r.ruleDirPrefixes[string(dir.AppendTo(r.nameBuf[:0]))]
		if !isPrefix {
			r.addExisting(cmp.Or(pchild, mchild).Data())

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
			node:   mchild,
			sm:     r.sm,
			dir:    dir,
			rules:  r.rules,
			parent: r,
		}

		if !hasRules {
			var rchild tree.Node

			if pchild == nil {
				rchild = &RuleLessDir{
					rulesDir:        rd,
					ruleDirPrefixes: r.ruleDirPrefixes,
					nameBuf:         r.nameBuf,
					rules:           new(RulesDir),
				}
			} else {
				rchild = &RuleLessDirPatch{
					rulesDir:        rd,
					ruleDirPrefixes: r.ruleDirPrefixes,
					previousRules:   pchild,
					nameBuf:         r.nameBuf,
				}
			}

			if !yield(name, rchild) {
				return
			}

			continue
		}

		rules := &RulesDir{
			rulesDir: rd,
		}

		if !yield(name, rules) {
			return
		}
	}
}
