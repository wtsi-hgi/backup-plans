package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"os"
	"strings"

	"golang.org/x/sys/unix"
	"vimagination.zapto.org/tree"
)

func (s *Server) Tree(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.tree)
}

func (s *Server) tree(w http.ResponseWriter, r *http.Request) error {
	dir, err := getDir(r)
	if err != nil {
		return err
	}

	s.treeMu.RLock()
	defer s.treeMu.RUnlock()

	var chosen Node = &s.structure

	for part := range pathParts(dir[1:]) {
		var err error

		if chosen, err = chosen.Child(part); err != nil {
			return ErrNotFound
		}
	}

	children := make(map[string]Summary)

	for childDir, child := range chosen.Children() {
		if strings.HasSuffix(childDir, "/") {
			children[childDir] = child.Summary()
		}
	}

	w.Header().Set("Content-Type", "application/json")

	return json.NewEncoder(w).Encode(children)
}

func (s *Server) AddTree(file string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}

	stat, err := f.Stat()
	if err != nil {
		return err
	}

	data, err := unix.Mmap(int(f.Fd()), 0, int(stat.Size()), unix.PROT_READ, unix.MAP_POPULATE|unix.MAP_SHARED)
	if err != nil {
		return err
	}

	db, err := tree.OpenMem(data)
	if err != nil {
		return fmt.Errorf("error opening tree: %w", err)
	}

	if db.NumChildren() != 1 {
		return ErrInvalidDatabase
	}

	var (
		rootPath string
		treeRoot tree.Node
	)

	db.Children()(func(path string, node tree.Node) bool {
		rootPath = path
		treeRoot = node

		return false
	})

	if !strings.HasPrefix(rootPath, "/") || !strings.HasSuffix(rootPath, "/") {
		return ErrInvalidRoot
	}

	s.treeMu.Lock()
	defer s.treeMu.Unlock()

	p := &s.structure

	for part := range pathParts(rootPath[1 : len(rootPath)-1]) {
		np, ok := p.children[part]
		if !ok {
			np = newTopLevelDir(p)

			p.SetChild(part, np)
		}

		dir, ok := np.(*TopLevelDir)
		if !ok {
			return ErrDeepTree
		}

		p = dir
	}

	name := rootPath[strings.LastIndexByte(rootPath[:len(rootPath)-1], '/')+1:]

	if existing, ok := p.children[name]; ok {
		if _, ok = existing.(*WrappedNode); !ok {
			return ErrDeepTree
		}
	}

	p.SetChild(name, &WrappedNode{MemTree: treeRoot.(*tree.MemTree)})

	if existing, ok := s.maps[rootPath]; ok {
		existing()
	}

	s.maps[rootPath] = func() {
		unix.Munmap(data)
		f.Close()
	}

	return nil
}

func pathParts(path string) iter.Seq[string] {
	return func(yield func(string) bool) {
		for {
			pos := strings.IndexByte(path, '/')
			if pos == -1 {
				return
			}

			if !yield(path[:pos+1]) {
				break
			}

			path = path[pos+1:]
		}
	}
}

var (
	ErrInvalidDatabase = errors.New("tree database should have a single root child")
	ErrInvalidRoot     = errors.New("invalid root child")
	ErrDeepTree        = errors.New("tree cannot be child of another tree")
	ErrNotFound        = Error{
		Code: http.StatusNotFound,
		Err:  errors.New("404 page not found"),
	}
)
