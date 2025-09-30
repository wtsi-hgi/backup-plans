package treegen

import (
	"context"
	"errors"
	"io"
	"iter"
	"slices"

	"github.com/wtsi-hgi/wrstat-ui/summary"
	"vimagination.zapto.org/byteio"
	"vimagination.zapto.org/tree"
)

func NewTree(w io.Writer) summary.OperationGenerator {
	ctx, cancel := context.WithCancelCause(context.Background())
	next := newNode(ctx)
	next.top = true

	go func(node *Node) {
		cancel(tree.Serialise(w, node))
	}(next)

	return func() summary.Operation {
		curr := next

		next = newNode(ctx)
		curr.child = next

		return curr
	}
}

type IDData struct {
	ID uint32
	*Meta
}

type NameChild struct {
	Name  string
	Child tree.Node
}

type Node struct {
	ctx context.Context

	path  *summary.DirectoryPath
	child *Node

	yield  chan NameChild
	writer chan *byteio.StickyLittleEndianWriter

	Directory

	top bool
}

func newNode(ctx context.Context) *Node {
	return &Node{
		ctx:    ctx,
		yield:  make(chan NameChild),
		writer: make(chan *byteio.StickyLittleEndianWriter),
		Directory: Directory{
			Users:  make(IDMeta),
			Groups: make(IDMeta),
		},
	}
}

type Directory struct {
	UID, GID uint32
	MTime    int64

	Users  IDMeta
	Groups IDMeta
}

func (d *Directory) Add(uid, gid uint32, mtime, size int64) {
	d.Users.Add(uid, mtime, size)
	d.Groups.Add(gid, mtime, size)
}

func (d *Directory) WriteTo(w io.Writer) (int64, error) {
	sw := byteio.StickyLittleEndianWriter{Writer: w}

	sw.WriteUintX(uint64(d.UID))
	sw.WriteUintX(uint64(d.GID))
	sw.WriteUint8(1) // 1 rule
	sw.WriteUint8(0) // rule ID 0 (warn)
	writeIDTimes(&sw, getSortedIDTimes(d.Users))
	writeIDTimes(&sw, getSortedIDTimes(d.Groups))

	return sw.Count, sw.Err
}

type Meta struct {
	MTime uint64
	Files uint64
	Bytes uint64
}

type IDMeta map[uint32]*Meta

func (i IDMeta) Add(id uint32, t, size int64) {
	existing, ok := i[id]
	if !ok {
		existing = new(Meta)

		i[id] = existing
	}

	existing.MTime = max(existing.MTime, uint64(t))
	existing.Files++
	existing.Bytes += uint64(size)
}

func (n *Node) Add(info *summary.FileInfo) error {
	if n.path == nil {
		n.path = info.Path
		n.UID = info.UID
		n.GID = info.GID
		n.MTime = info.MTime

		if n.top {
			if err := n.sendChild(n.path.AppendTo(nil), n); err != nil {
				return err
			}
		}
	} else if info.Path.Parent == n.path && info.IsDir() {
		if err := n.sendChild(info.Name, n.child); err != nil {
			return err
		}
	} else if info.Path == n.path {
		if err := n.sendChild(info.Name, &File{info.UID, info.GID, info.MTime, info.Size}); err != nil {
			return err
		}
	}

	if !info.IsDir() {
		n.Directory.Add(info.UID, info.GID, info.MTime, info.Size)
	}

	return nil
}

func (n *Node) sendChild(name []byte, child tree.Node) error {
	select {
	case <-n.ctx.Done():
		return context.Cause(n.ctx)
	case n.yield <- NameChild{Name: string(name), Child: child}:
		return nil
	}
}

func (n *Node) Output() error {
	close(n.yield)

	select {
	case <-n.ctx.Done():
		return context.Cause(n.ctx)
	case w := <-n.writer:
		n.Directory.WriteTo(w)

		n.writer <- nil

		if w.Err != nil {
			return w.Err
		}
	}

	var err error

	if n.top {
		select {
		case <-n.ctx.Done():
		case <-n.writer:
			n.writer <- nil

			<-n.ctx.Done()

			if err = context.Cause(n.ctx); errors.Is(err, context.Canceled) {
				err = nil
			}
		}
	}

	n.path = nil
	clear(n.Users)
	clear(n.Groups)

	return err
}

func (n *Node) Children() iter.Seq2[string, tree.Node] {
	return func(yield func(string, tree.Node) bool) {
		for nc := range n.yield {
			if !yield(nc.Name, nc.Child) {
				return
			}
		}

		n.yield = make(chan NameChild)
	}
}

func (n *Node) WriteTo(w io.Writer) (int64, error) {
	lw := &byteio.StickyLittleEndianWriter{Writer: w}

	n.writer <- lw
	<-n.writer

	return lw.Count, lw.Err
}

func getSortedIDTimes(idt IDMeta) []IDData {
	var idts []IDData

	for id, meta := range idt {
		it := IDData{id, meta}

		idx, _ := slices.BinarySearchFunc(idts, it, func(a, b IDData) int {
			return int(a.ID) - int(b.ID)
		})

		idts = slices.Insert(idts, idx, it)
	}

	return idts
}

func writeIDTimes(w *byteio.StickyLittleEndianWriter, idts []IDData) {
	w.WriteUintX(uint64(len(idts)))

	for _, idt := range idts {
		w.WriteUintX(uint64(idt.ID))
		w.WriteUintX(uint64(idt.MTime))
		w.WriteUintX(idt.Files)
		w.WriteUintX(idt.Bytes)
	}
}

type File struct {
	UID, GID    uint32
	MTime, Size int64
}

func (f *File) WriteTo(w io.Writer) (int64, error) {
	sw := byteio.StickyLittleEndianWriter{Writer: w}

	sw.WriteUintX(uint64(f.UID))
	sw.WriteUintX(uint64(f.GID))
	sw.WriteUintX(uint64(f.MTime))
	sw.WriteUintX(uint64(f.Size))

	return sw.Count, sw.Err
}

func (File) Children() iter.Seq2[string, tree.Node] {
	return func(_ func(string, tree.Node) bool) {}
}
