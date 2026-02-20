package backups

import (
	"bufio"
	"errors"
	"fmt"
	"iter"
	"os"
	"strings"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	"github.com/wtsi-hgi/backup-plans/ruletree"
	"github.com/wtsi-hgi/wrstat-ui/summary"
	"github.com/wtsi-hgi/wrstat-ui/summary/group"
	"vimagination.zapto.org/tree"
)

const (
	setNamePrefix = "plan::"
)

var hasBackups int64 = -1 //nolint:gochecknoglobals

type SetInfo struct {
	BackupSetName string
	Requestor     string
	FileCount     int
}

// Backup will back up all files in the given treeNode that match rules in the
// given planDB, using the given ibackup client. It returns a list of the set IDs
// created.
func Backup(planDB *db.DB, treeNode tree.Node, client *ibackup.MultiClient) ([]SetInfo, error) {
	setFofns, err := collectSetFofns(planDB, treeNode)
	if err != nil {
		return nil, err
	}

	defer closeStreams(setFofns)

	return addFofnsToIBackup(client, setFofns)
}

// BackupWithFofnWriter will back up all files in the given treeNode that match
// rules in the given planDB, using the given ibackup client and fofn writer.
// It returns a list of set IDs created.
func BackupWithFofnWriter(planDB *db.DB, treeNode tree.Node,
	client *ibackup.MultiClient,
	newFofnDirWriter func(baseDir string) *ibackup.FofnDirWriter) ([]SetInfo, error) {
	_ = newFofnDirWriter

	return Backup(planDB, treeNode, client)
}

func collectSetFofns(planDB *db.DB, treeNode tree.Node) (map[*db.Directory]*setStream, error) {
	mountpoint, err := readMountpoint(treeNode)
	if err != nil {
		return nil, err
	}

	sm, dirRulesByID, err := buildBackupState(planDB, mountpoint)
	if err != nil {
		return nil, err
	}

	setFofns := make(map[*db.Directory]*setStream)

	figureOutFOFNs(treeNode, sm, nil, func(path *summary.DirectoryPath, ruleID int64) {
		rule := dirRulesByID[ruleID]

		if rule.RuleIDs[ruleID].BackupType != db.BackupIBackup {
			return
		}

		stream := setFofns[rule.Directory]
		if stream == nil {
			stream, err = newSetStream()
			if err != nil {
				return
			}

			setFofns[rule.Directory] = stream
		}

		err = stream.Append(string(path.AppendTo(nil)))
		if err != nil {
			return
		}
	})

	if err != nil {
		closeStreams(setFofns)

		return nil, err
	}

	for _, stream := range setFofns {
		if cerr := stream.Finish(); cerr != nil {
			closeStreams(setFofns)

			return nil, cerr
		}
	}

	return setFofns, nil
}

func readMountpoint(treeNode tree.Node) (string, error) {
	var (
		mountpoint string
		err        error
	)

	treeNode.Children()(func(name string, n tree.Node) bool {
		if cerr, ok := n.(tree.ChildrenError); ok {
			err = cerr.Unwrap()
		}

		mountpoint = name

		return false
	})

	return mountpoint, err
}

func buildBackupState(planDB *db.DB, mountpoint string) (ruletree.State,
	map[int64]*dirRules, error) {
	dirs, dirRulesByID, err := readDirRules(planDB, mountpoint)
	if err != nil {
		return ruletree.State{}, nil, err
	}

	root := ruletree.NewRuleTree()

	for _, dr := range dirs {
		root.Set(dr.Path, dr.Rules, false)
	}

	root.Canon()
	root.MarkBackupDirs()

	rules := root.BuildRules()
	rules[0] = collectRuleGroups(root, "/", rules[0])

	sm, err := ruletree.BuildMultiStateMachine(rules)
	if err != nil {
		return ruletree.State{}, nil, err
	}

	return sm, dirRulesByID, nil
}

func readDirRules(planDB *db.DB, mountpoint string) (map[int64]*dirRules, map[int64]*dirRules, error) {
	dirs := make(map[int64]*dirRules)
	rules := make(map[int64]*dirRules)

	if err := planDB.ReadDirectories().ForEach(func(dir *db.Directory) error {
		if strings.HasPrefix(dir.Path, mountpoint) {
			dirs[dir.ID()] = &dirRules{
				Directory: dir,
				Rules:     make(map[string]*db.Rule),
				RuleIDs:   make(map[int64]*db.Rule),
			}
		}

		return nil
	}); err != nil {
		return nil, nil, err
	}

	if err := planDB.ReadRules().ForEach(func(rule *db.Rule) error {
		if dir, ok := dirs[rule.DirID()]; ok {
			dir.Rules[rule.Match] = rule
			dir.RuleIDs[rule.ID()] = rule
			rules[rule.ID()] = dir
		}

		return nil
	}); err != nil {
		return nil, nil, err
	}

	return dirs, rules, nil
}

func collectRuleGroups(root *ruletree.RuleTree, path string, rules ruletree.Rules) ruletree.Rules {
	if !root.HasBackup && !root.HasChildWithBackup {
		return rules
	}

	rules = append(rules, group.PathGroup[int64]{
		Path:  []byte(path),
		Group: &hasBackups,
	})

	if root.HasBackup {
		rules = append(rules, group.PathGroup[int64]{
			Path:  []byte(path + "*/"),
			Group: &hasBackups,
		})
	}

	for name, rt := range root.Iter() {
		rules = collectRuleGroups(rt, path+name, rules)
	}

	return rules
}

func figureOutFOFNs(node tree.Node, sm ruletree.State, path *summary.DirectoryPath,
	cb func(*summary.DirectoryPath, int64)) {
	for name, child := range node.Children() {
		state := sm.GetStateString(name)
		newPath := &summary.DirectoryPath{Parent: path, Name: name}
		group := state.GetGroup()

		if group == nil {
			continue
		}

		if !strings.HasSuffix(name, "/") {
			cb(newPath, *group)

			continue
		}

		if *group != hasBackups {
			continue
		}

		figureOutFOFNs(child, state, newPath, cb)
	}
}

func addFofnsToIBackup(client *ibackup.MultiClient,
	setFofns map[*db.Directory]*setStream) ([]SetInfo, error) {
	backupSetInfos := make([]SetInfo, 0, len(setFofns))

	var errs error

	for setInfo, stream := range setFofns {
		backupSetName := setNamePrefix + setInfo.Path

		err := client.BackupStream(setInfo.Path, backupSetName, setInfo.ClaimedBy,
			stream.Seq(), int(setInfo.Frequency), setInfo.ReviewDate,
			setInfo.RemoveDate) //nolint:gosec
		if err != nil {
			errs = errors.Join(errs, err)

			continue
		}

		backupSetInfos = append(backupSetInfos, SetInfo{
			BackupSetName: backupSetName,
			Requestor:     setInfo.ClaimedBy,
			FileCount:     stream.Count(),
		})
	}

	return backupSetInfos, errs
}

func closeStreams(streams map[*db.Directory]*setStream) {
	for _, stream := range streams {
		stream.Close()
	}
}

type setStream struct {
	path   string
	count  int
	fd     *os.File
	closed bool
}

func newSetStream() (*setStream, error) {
	fd, err := os.CreateTemp("", "backup-plans-stream-*")
	if err != nil {
		return nil, err
	}

	return &setStream{path: fd.Name(), fd: fd}, nil
}

func (s *setStream) Append(path string) error {
	if s.fd == nil {
		return fmt.Errorf("set stream is closed")
	}

	_, err := s.fd.WriteString(path)
	if err != nil {
		return err
	}

	_, err = s.fd.WriteString("\n")
	if err != nil {
		return err
	}

	s.count++

	return nil
}

func (s *setStream) Finish() error {
	if s.fd == nil {
		return nil
	}

	err := s.fd.Close()
	s.fd = nil

	return err
}

func (s *setStream) Seq() iter.Seq[string] {
	return func(yield func(string) bool) {
		fd, err := os.Open(s.path)
		if err != nil {
			return
		}

		defer fd.Close()

		scanner := bufio.NewScanner(fd)
		scanner.Buffer(make([]byte, 0, 1024), 1024*1024)

		for scanner.Scan() {
			if !yield(scanner.Text()) {
				return
			}
		}
	}
}

func (s *setStream) Count() int {
	return s.count
}

func (s *setStream) Close() {
	if s.closed {
		return
	}

	if s.fd != nil {
		s.fd.Close()
		s.fd = nil
	}

	os.Remove(s.path)
	s.closed = true
}

type dirRules struct {
	*db.Directory
	Rules   map[string]*db.Rule
	RuleIDs map[int64]*db.Rule
}
