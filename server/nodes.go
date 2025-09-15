package server

import (
	"bytes"
	"iter"

	"github.com/wtsi-hgi/backup-plans/users"
	"vimagination.zapto.org/byteio"
	"vimagination.zapto.org/tree"
)

type Node interface {
	Child(string) (Node, error)
	Children() iter.Seq2[string, Node]
	Summary() Summary
}

type Summary struct {
	UID, GID      uint32
	Users, Groups map[string]Stats
}

type SizeCount struct {
	Size  int64
	Count int64
}

type TopLevelDir struct {
	children map[string]Node
	summary  Summary
}

func (t *TopLevelDir) Child(name string) (Node, error) {
	child, ok := t.children[name]
	if !ok {
		return nil, tree.ChildNotFoundError(name)
	}

	return child, nil
}

func (t *TopLevelDir) Children() iter.Seq2[string, Node] {
	return func(yield func(string, Node) bool) {
		for name, child := range t.children {
			if !yield(name, child) {
				return
			}
		}
	}
}

func (t *TopLevelDir) Summary() Summary {
	return t.summary
}

type Stats struct {
	ID    uint32
	MTime uint32
	Files uint32
	Size  uint64
}

func readStats(br byteio.StickyEndianReader) iter.Seq[Stats] {
	return func(yield func(Stats) bool) {
		for range br.ReadUintX() {
			if !yield(Stats{
				ID:    uint32(br.ReadUintX()),
				MTime: uint32(br.ReadUintX()),
				Files: uint32(br.ReadUintX()),
				Size:  br.ReadUintX(),
			}) {
				return
			}
		}
	}
}

type WrappedNode struct {
	*tree.MemTree
}

func (w *WrappedNode) Child(name string) (Node, error) {
	child, err := w.MemTree.Child(name)
	if err != nil {
		return nil, err
	}

	return &WrappedNode{MemTree: child}, nil
}

func (w *WrappedNode) Children() iter.Seq2[string, Node] {
	return func(yield func(string, Node) bool) {
		for name, child := range w.MemTree.Children() {
			if !yield(name, &WrappedNode{MemTree: child.(*tree.MemTree)}) {
				return
			}
		}
	}
}

func (w *WrappedNode) Summary() Summary {
	userStats := make(map[string]Stats)
	groupStats := make(map[string]Stats)

	br := byteio.StickyLittleEndianReader{Reader: bytes.NewReader(w.Data())}

	uid := br.ReadUintX()
	gid := br.ReadUintX()

	for user := range readStats(&br) {
		userStats[users.Username(user.ID)] = user
	}

	for group := range readStats(&br) {
		groupStats[users.Group(group.ID)] = group
	}

	return Summary{
		UID:    uint32(uid),
		GID:    uint32(gid),
		Users:  userStats,
		Groups: groupStats,
	}
}
