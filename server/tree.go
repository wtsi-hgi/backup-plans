package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"maps"
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

func (s *Server) AddTree(file string) (err error) {
	db, closer, err := openDB(file)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			closer()
		}
	}()

	treeRoot, rootPath, err := getRoot(db)
	if err != nil {
		return err
	}

	s.treeMu.Lock()
	defer s.treeMu.Unlock()

	if err = s.processRules(treeRoot, rootPath); err != nil {
		return err
	}

	if err = createTopLevelDirs(treeRoot, rootPath, &s.structure); err != nil {
		return err
	}

	if existing, ok := s.maps[rootPath]; ok {
		existing()
	}

	s.maps[rootPath] = closer

	return nil
}

func openDB(file string) (*tree.MemTree, func(), error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, nil, err
	}

	stat, err := f.Stat()
	if err != nil {
		f.Close()

		return nil, nil, err
	}

	data, err := unix.Mmap(int(f.Fd()), 0, int(stat.Size()), unix.PROT_READ, unix.MAP_POPULATE|unix.MAP_SHARED)
	if err != nil {
		f.Close()

		return nil, nil, err
	}

	fn := func() {
		unix.Munmap(data)
		f.Close()
	}

	db, err := tree.OpenMem(data)
	if err != nil {
		fn()

		return nil, nil, fmt.Errorf("error opening tree: %w", err)
	}

	return db, fn, nil
}

func getRoot(db *tree.MemTree) (*tree.MemTree, string, error) {
	if db.NumChildren() != 1 {
		return nil, "", ErrInvalidDatabase
	}

	var (
		rootPath string
		treeRoot *tree.MemTree
	)

	db.Children()(func(path string, node tree.Node) bool {
		rootPath = path
		treeRoot = node.(*tree.MemTree)

		return false
	})

	if !strings.HasPrefix(rootPath, "/") || !strings.HasSuffix(rootPath, "/") {
		return nil, "", ErrInvalidRoot
	}

	return treeRoot, rootPath, nil
}

func (s *Server) processRules(treeRoot *tree.MemTree, rootPath string) error {
	// 	rulePrefixes := s.createRulePrefixMap(rootPath)

	// 	var childErr tree.ChildNotFoundError

	// 	rd := RulesDir{
	// 		sm: s.stateMachine,
	// 		dir: summary.DirectoryPath{Name: rootPath},
	// 	}

	// Loop:
	// 	for {
	// 		rd.node = treeRoot

	// 		for part := range pathParts(ruleDir) {
	// 			rd.node, err = rd.node.Child(part)
	// 			if errors.As(err, &childErr) {
	// 				break Loop
	// 			} else if err != nil {
	// 				return err
	// 			}
	// 		}

	// 		lastRule = ruleDir

	// 		rd.dir.Name = ruleDir
	// 	}

	return nil
}

func (s *Server) createRulePrefixMap(rootPath string) map[string]bool {
	rulePrefixes := make(map[string]bool)

	for ruleDir := range maps.Keys(s.rules) {
		if !strings.HasPrefix(ruleDir, rootPath) {
			continue
		}

		rulePrefixes[ruleDir] = true

		for ruleDir != "/" {
			ruleDir = ruleDir[:strings.LastIndexByte(ruleDir, '/')+1]
			rulePrefixes[ruleDir] = rulePrefixes[ruleDir] || false
		}
	}

	return rulePrefixes
}

func createTopLevelDirs(treeRoot *tree.MemTree, rootPath string, p *TopLevelDir) error {
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

	p.SetChild(name, &WrappedNode{MemTree: treeRoot})

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
