package server

import (
	"bytes"
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
	sw.WriteUintX(uint64(r.ID))
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
		addUserWarn(uid uint32, mtime, files, bytes uint64)
		addGroupWarn(gid uint32, mtime, files, bytes uint64)
	}
}

type RulesDir struct {
	rulesDir

	child *RulesDir
}

func (r *RulesDir) Children() iter.Seq2[string, tree.Node] {
	if r.child == nil {
		r.child = &RulesDir{
			rulesDir: rulesDir{
				sm:     r.sm,
				parent: r,
			},
		}
	}

	r.child.dir.Parent = &r.dir
	r.rules = r.rules[:0]

	return r.children
}

func (r *RulesDir) children(yield func(string, tree.Node) bool) {
	var f file

	for name, child := range r.node.Children() {
		mchild := child.(*tree.MemTree)

		if strings.HasSuffix(name, "/") {
			r.child.dir.Name = name
			r.child.node = mchild

			if !yield(name, r.child) {
				return
			}
		} else {
			rule := r.sm.GetGroup(&summary.FileInfo{Path: &r.dir, Name: fileName(name)})

			if _, err := f.ReadFrom(bytes.NewReader(mchild.Data())); err != nil {
				yield(name, tree.NewChildrenError(err))

				return
			}

			r.setRule(rule, &f)
		}
	}
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

	pos := r.getRulePos(rule)

	r.rules[pos].Users.add(f.uid, f.mtime, 1, f.size)
	r.rules[pos].Groups.add(f.gid, f.mtime, 1, f.size)
}

func (r *rulesDir) getRulePos(rule *db.Rule) int {
	newRule := Rule{ID: uint64(rule.ID())}

	pos, ok := slices.BinarySearchFunc(r.rules, newRule, func(a, b Rule) int {
		return int(a.ID - b.ID)
	})
	if !ok {
		r.rules = slices.Insert(r.rules, pos, newRule)
	}

	return pos
}

func (r *rulesDir) addUserWarn(uid uint32, mtime, files, bytes uint64) {
	if r.parent != nil {
		r.parent.addUserWarn(uid, mtime, files, bytes)
	}

	pos := r.getRulePos(nil)

	r.rules[pos].Users.add(uid, mtime, files, bytes)
}

func (r *rulesDir) addGroupWarn(gid uint32, mtime, files, bytes uint64) {
	if r.parent != nil {
		r.parent.addGroupWarn(gid, mtime, files, bytes)
	}

	pos := r.getRulePos(nil)

	r.rules[pos].Groups.add(gid, mtime, files, bytes)
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

	child   *RuleLessDir
	rules   *RulesDir
	nameBuf *[4096]byte
}

func (r *RuleLessDir) Children() iter.Seq2[string, tree.Node] {
	if r.child == nil {
		r.child = &RuleLessDir{
			rulesDir: rulesDir{
				sm:     r.sm,
				parent: r,
			},
			rules:           r.rules,
			ruleDirPrefixes: r.ruleDirPrefixes,
			nameBuf:         r.nameBuf,
		}
	}

	r.child.dir.Parent = &r.dir
	r.rulesDir.rules = r.rulesDir.rules[:0]

	return r.children
}

func (r *RuleLessDir) children(yield func(string, tree.Node) bool) {
	for name, child := range r.node.Children() {
		if !strings.HasSuffix(name, "/") {
			continue
		}

		r.child.dir.Name = name
		mchild := child.(*tree.MemTree)

		hasRules, isPrefix := r.ruleDirPrefixes[string(r.child.dir.AppendTo(r.nameBuf[:0]))]
		if !isPrefix {
			r.addWarn(mchild.Data())

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

func (r *RuleLessDir) addWarn(data []byte) {
	sr := byteio.StickyLittleEndianReader{Reader: bytes.NewReader(data)}

	sr.ReadUintX()
	sr.ReadUintX()
	sr.ReadUint8()
	sr.ReadUint8()

	readArray(&sr, r.addUserWarn)
	readArray(&sr, r.addGroupWarn)
}

func readArray(sr *byteio.StickyLittleEndianReader, fn func(uint32, uint64, uint64, uint64)) {
	for range sr.ReadUintX() {
		uid := uint32(sr.ReadUintX())
		mtime := sr.ReadUintX()
		files := sr.ReadUintX()
		bytes := sr.ReadUintX()

		fn(uid, mtime, files, bytes)
	}
}
