package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"os"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
	"vimagination.zapto.org/tree"
)

type Server struct {
	mu        sync.RWMutex
	maps      map[string]func()
	structure TopLevelDir
}

func New() *Server {
	return &Server{
		maps: make(map[string]func()),
		structure: TopLevelDir{
			children: map[string]Node{},
		},
	}
}

func (s *Server) Tree(w http.ResponseWriter, r *http.Request) {
	dir := r.FormValue("dir")

	if !strings.HasSuffix(dir, "/") || !strings.HasPrefix(dir, "/") {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	s.mu.RLock()

	var chosen Node = &s.structure

	for part := range pathParts(dir[1:]) {
		var err error

		chosen, err = chosen.Child(part)
		if err != nil {
			s.mu.RUnlock()
			http.NotFound(w, r)

			return
		}
	}

	var paths []string

	for childDir := range chosen.Children() {
		if strings.HasSuffix(childDir, "/") {
			paths = append(paths, childDir)
		}
	}

	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(struct {
		Summary  Summary
		Children []string
	}{
		Summary:  chosen.Summary(),
		Children: paths,
	})
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

	s.mu.Lock()
	defer s.mu.Unlock()

	p := &s.structure

	for part := range pathParts(rootPath[1 : len(rootPath)-1]) {
		np, ok := p.children[part]
		if !ok {
			np = &TopLevelDir{children: map[string]Node{}}

			p.children[part] = np
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

	p.children[name] = &WrappedNode{MemTree: treeRoot.(*tree.MemTree)}

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
)
