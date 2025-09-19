package server

import (
	"io"
	"iter"
	"slices"
	"strings"
	"unsafe"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/wrstat-ui/summary"
	"github.com/wtsi-hgi/wrstat-ui/summary/group"
	"vimagination.zapto.org/byteio"
	"vimagination.zapto.org/memio"
	"vimagination.zapto.org/tree"
)

type Rule struct {
	*db.Rule
	users, groups ruleStats
}

func (r *Rule) writeTo(sw *byteio.StickyLittleEndianWriter) {
	sw.WriteUintX(uint64(r.ID()))

	sw.WriteUintX(uint64(len(r.users)))

	for n := range r.users {
		r.users[n].writeTo(sw)
	}

	sw.WriteUintX(uint64(len(r.groups)))

	for n := range r.groups {
		r.groups[n].writeTo(sw)
	}
}

type ruleStats []Stats

func (r *ruleStats) add(id uint32, mtime uint32, size uint64) {
	newStats := Stats{
		ID: id,
	}

	pos, ok := slices.BinarySearchFunc(*r, newStats, func(a, b Stats) int {
		return int(a.ID) - int(b.ID)
	})
	if !ok {
		*r = slices.Insert(*r, pos, newStats)
	}

	(*r)[pos].MTime = max((*r)[pos].MTime, mtime)
	(*r)[pos].Files++
	(*r)[pos].Size += size
}

type RulesDir struct {
	node *tree.MemTree
	sm   group.StateMachine[db.Rule]
	dir  summary.DirectoryPath

	parent *RulesDir
	child  *RulesDir
	rules  []Rule
}

func (r *RulesDir) Children() iter.Seq2[string, tree.Node] {
	if r.child == nil {
		r.child = &RulesDir{
			sm: r.sm,
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
			data := memio.Buffer(mchild.Data())

			if _, err := f.ReadFrom(&data); err != nil {
				yield(name, tree.NewChildrenError(err))

				return
			}

			r.setRule(rule, &f)
		}
	}
}

func (r *RulesDir) WriteTo(w io.Writer) (int64, error) {
	sw := byteio.StickyLittleEndianWriter{Writer: w}

	sw.WriteUintX(uint64(len(r.rules)))

	for n := range r.rules {
		r.rules[n].writeTo(&sw)
	}

	return sw.Count, sw.Err
}

func (r *RulesDir) setRule(rule *db.Rule, f *file) {
	if r.parent != nil {
		r.parent.setRule(rule, f)
	}

	newRule := Rule{Rule: rule}

	pos, ok := slices.BinarySearchFunc(r.rules, newRule, func(a, b Rule) int {
		return int(a.ID() - b.ID())
	})
	if !ok {
		r.rules = slices.Insert(r.rules, pos, newRule)
	}

	r.rules[pos].users.add(f.uid, f.mtime, f.size)
	r.rules[pos].groups.add(f.gid, f.mtime, f.size)
}

func fileName(str string) []byte {
	return unsafe.Slice(unsafe.StringData(str), len(str))
}

type file struct {
	uid, gid uint32
	mtime    uint32
	size     uint64
}

func (f *file) ReadFrom(r io.Reader) (int64, error) {
	lr := byteio.StickyLittleEndianReader{Reader: r}

	f.uid = uint32(lr.ReadUintX())
	f.gid = uint32(lr.ReadUintX())
	f.mtime = uint32(lr.ReadUintX())
	f.size = uint64(lr.ReadUintX())

	return lr.Count, lr.Err
}
