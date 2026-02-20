package backups

import (
	"errors"
	"math"
	"strconv"
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
	intSize32     = 32
	intSize64     = 64
)

var hasBackups int64 = -1 //nolint:gochecknoglobals

var errInvalidFrequency = errors.New("frequency overflows int")

type SetInfo struct {
	BackupSetName string
	Requestor     string
	FileCount     int
}

// Backup will back up all files in the given treeNode that match rules in the
// given planDB, using the given ibackup client. It returns a list of the set IDs
// created.
//
// For fofn-backed servers, file paths stream directly to the fofn file
// without intermediate temp files. For API-backed servers, paths are
// collected in memory as a slice.
func Backup(planDB *db.DB, treeNode tree.Node, client *ibackup.MultiClient) ([]SetInfo, error) {
	mountpoint, err := readMountpoint(treeNode)
	if err != nil {
		return nil, err
	}

	sm, dirRulesByID, err := buildBackupState(planDB, mountpoint)
	if err != nil {
		return nil, err
	}

	writers := make(map[*db.Directory]*setWriter)

	figureOutFOFNs(treeNode, sm, nil, func(path *summary.DirectoryPath, ruleID int64) {
		if err == nil {
			err = addToWriter(writers, dirRulesByID, client, path, ruleID)
		}
	})

	if err != nil {
		closeWriters(writers)

		return nil, err
	}

	return finishWriters(writers)
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

func addToWriter(writers map[*db.Directory]*setWriter,
	dirRulesByID map[int64]*dirRules, client *ibackup.MultiClient,
	path *summary.DirectoryPath, ruleID int64) error {
	rule := dirRulesByID[ruleID]

	if rule.RuleIDs[ruleID].BackupType != db.BackupIBackup {
		return nil
	}

	sw := writers[rule.Directory]
	if sw == nil {
		var err error

		sw, err = newSetWriter(rule, client)
		if err != nil {
			return err
		}

		writers[rule.Directory] = sw
	}

	return sw.Add(string(path.AppendTo(nil)))
}

func newSetWriter(rule *dirRules, client *ibackup.MultiClient) (*setWriter, error) {
	freq, err := uintToInt(rule.Frequency)
	if err != nil {
		return nil, err
	}

	w, err := client.NewSetWriter(
		rule.Path,
		setNamePrefix+rule.Path,
		rule.ClaimedBy,
		freq,
		rule.ReviewDate,
		rule.RemoveDate,
	)
	if err != nil {
		return nil, err
	}

	return &setWriter{SetWriter: w, dir: rule.Directory}, nil
}

func uintToInt(value uint) (int, error) {
	v := uint64(value)

	if strconv.IntSize == intSize32 && v > math.MaxInt32 {
		return 0, errInvalidFrequency
	}

	if strconv.IntSize == intSize64 && v > math.MaxInt64 {
		return 0, errInvalidFrequency
	}

	if strconv.IntSize == intSize32 {
		return int(int32(v)), nil
	}

	return int(int64(v)), nil
}

func closeWriters(writers map[*db.Directory]*setWriter) {
	for _, w := range writers {
		w.Close()
	}
}

func finishWriters(writers map[*db.Directory]*setWriter) ([]SetInfo, error) {
	infos := make([]SetInfo, 0, len(writers))

	var errs error

	for _, w := range writers {
		if err := w.Finish(); err != nil {
			errs = errors.Join(errs, err)

			continue
		}

		infos = append(infos, SetInfo{
			BackupSetName: setNamePrefix + w.dir.Path,
			Requestor:     w.dir.ClaimedBy,
			FileCount:     w.Count(),
		})
	}

	return infos, errs
}

type setWriter struct {
	*ibackup.SetWriter
	dir *db.Directory
}

type dirRules struct {
	*db.Directory
	Rules   map[string]*db.Rule
	RuleIDs map[int64]*db.Rule
}
