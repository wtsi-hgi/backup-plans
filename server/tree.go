package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"os"
	"strings"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/wrstat-ui/summary"
	"golang.org/x/sys/unix"
	"vimagination.zapto.org/tree"
)

type Tree struct {
	*DirSummary
	ClaimedBy string
	Rules     map[string]map[uint64]*db.Rule
}

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

	summary, err := s.structure.Summary(dir[1:])
	if err != nil {
		return err
	}

	t := Tree{
		DirSummary: summary,
		Rules:      make(map[string]map[uint64]*db.Rule),
	}

	dirRules, ok := s.directoryRules[dir]
	if ok {
		t.ClaimedBy = dirRules.ClaimedBy
		thisDir := make(map[uint64]*db.Rule)
		t.Rules[dir] = thisDir

		for _, rule := range dirRules.Rules {
			thisDir[uint64(rule.ID())] = rule
		}
	}

	for _, rs := range t.RuleSummaries {
		if rs.ID == 0 {
			continue
		}

		rule := s.rules[rs.ID]
		dir := s.dirs[uint64(rule.DirID())]

		r, ok := t.Rules[dir.Path]
		if !ok {
			r = make(map[uint64]*db.Rule)
			t.Rules[dir.Path] = r
		}

		r[rs.ID] = rule
	}

	w.Header().Set("Content-Type", "application/json")

	return json.NewEncoder(w).Encode(t)
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

	processed, err := s.processRules(treeRoot, rootPath)
	if err != nil {
		return err
	}

	if err = createTopLevelDirs(processed, rootPath, &s.structure); err != nil {
		return err
	}

	if existing, ok := s.closers[rootPath]; ok {
		existing()
	}

	s.closers[rootPath] = closer

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

func (s *Server) processRules(treeRoot *tree.MemTree, rootPath string) (*RuleOverlay, error) {
	rd := RuleLessDir{
		rulesDir: rulesDir{
			node: treeRoot,
			sm:   s.stateMachine,
			dir:  summary.DirectoryPath{Name: rootPath},
		},
		ruleDirPrefixes: s.createRulePrefixMap(rootPath),
		rules: &RulesDir{
			rulesDir: rulesDir{
				sm: s.stateMachine,
			},
		},
		nameBuf: new([4096]byte),
	}

	var buf bytes.Buffer

	if err := tree.Serialise(&buf, &rd); err != nil {
		return nil, err
	}

	processed, err := tree.OpenMem(buf.Bytes())
	if err != nil {
		return nil, err
	}

	return &RuleOverlay{lower: treeRoot, upper: processed}, nil
}

func (s *Server) createRulePrefixMap(rootPath string) map[string]bool {
	rulePrefixes := make(map[string]bool)

	for ruleDir := range s.directoryRules {
		if !strings.HasPrefix(ruleDir, rootPath) {
			continue
		}

		rulePrefixes[ruleDir] = true

		for ruleDir != "/" {
			ruleDir = ruleDir[:strings.LastIndexByte(ruleDir[:len(ruleDir)-1], '/')+1]
			rulePrefixes[ruleDir] = rulePrefixes[ruleDir] || false
		}
	}

	return rulePrefixes
}

func createTopLevelDirs(treeRoot *RuleOverlay, rootPath string, p *TopLevelDir) error {
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
		if _, ok = existing.(*RuleOverlay); !ok {
			return ErrDeepTree
		}
	}

	p.SetChild(name, treeRoot)

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
