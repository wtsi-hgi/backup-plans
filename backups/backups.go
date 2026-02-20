package backups

import (
	"errors"
	"iter"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	"github.com/wtsi-hgi/backup-plans/ruletree"
	"github.com/wtsi-hgi/ibackup/fofn"
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

func processSetBackup(client *ibackup.MultiClient, fofnWriters map[string]fofnDirWriter,
	writerFactory fofnDirWriterFactory, dir *db.Directory, files []string) (SetInfo, error) {
	backupSetName := setNamePrefix + dir.Path
	transformer := client.GetTransformer(dir.Path)

	err := writeFofnSetIfConfigured(fofnWriters, writerFactory,
		client.GetFofnDir(dir.Path), backupSetName, transformer, dir, files)
	if err != nil {
		return SetInfo{}, err
	}

	err = client.Backup(dir.Path, backupSetName, dir.ClaimedBy, files,
		int(dir.Frequency), dir.ReviewDate, dir.RemoveDate) //nolint:gosec
	if err != nil {
		return SetInfo{}, err
	}

	return SetInfo{
		BackupSetName: backupSetName,
		Requestor:     dir.ClaimedBy,
		FileCount:     len(files),
	}, nil
}

// Backup will back up all files in the given treeNode that match rules in the
// given planDB, using the given ibackup client. It returns a list of the set IDs
// created.
func Backup(planDB *db.DB, treeNode tree.Node, client *ibackup.MultiClient) ([]SetInfo, error) {
	return BackupWithFofnWriter(planDB, treeNode, client, ibackup.NewFofnDirWriter)
}

// BackupWithFofnWriter will back up all files in the given treeNode that match
// rules in the given planDB, using the given ibackup client and fofn writer.
// It returns a list of set IDs created.
func BackupWithFofnWriter(planDB *db.DB, treeNode tree.Node,
	client *ibackup.MultiClient,
	newFofnDirWriter func(baseDir string) *ibackup.FofnDirWriter) ([]SetInfo, error) {
	setFofns, err := collectSetFofns(planDB, treeNode)
	if err != nil {
		return nil, err
	}

	return addFofnsToIBackup(client, setFofns, resolveFofnWriterFactory(newFofnDirWriter))
}

func collectSetFofns(planDB *db.DB, treeNode tree.Node) (map[*db.Directory][]string, error) {
	mountpoint, err := readMountpoint(treeNode)
	if err != nil {
		return nil, err
	}

	sm, dirRulesByID, err := buildBackupState(planDB, mountpoint)
	if err != nil {
		return nil, err
	}

	setFofns := make(map[*db.Directory][]string)

	figureOutFOFNs(treeNode, sm, nil, func(path *summary.DirectoryPath, ruleID int64) {
		rule := dirRulesByID[ruleID]

		if rule.RuleIDs[ruleID].BackupType == db.BackupIBackup {
			setFofns[rule.Directory] = append(setFofns[rule.Directory], string(path.AppendTo(nil)))
		}
	})

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

func resolveFofnWriterFactory(newFofnDirWriter func(baseDir string) *ibackup.FofnDirWriter) fofnDirWriterFactory {
	if newFofnDirWriter == nil {
		return nil
	}

	return func(baseDir string) fofnDirWriter {
		return newFofnDirWriter(baseDir)
	}
}

func addFofnsToIBackup(client *ibackup.MultiClient, setFofns map[*db.Directory][]string,
	writerFactory fofnDirWriterFactory) ([]SetInfo, error) {
	backupSetInfos := make([]SetInfo, 0, len(setFofns))
	fofnWriters := make(map[string]fofnDirWriter)

	var errs error

	for setInfo, fofns := range setFofns {
		backupSetInfo, err := processSetBackup(client, fofnWriters,
			writerFactory, setInfo, fofns)
		if err != nil {
			errs = errors.Join(errs, err)

			continue
		}

		backupSetInfos = append(backupSetInfos, backupSetInfo)
	}

	return backupSetInfos, errs
}

type fofnDirWriter interface {
	Write(setName string, transformer string, files iter.Seq[string],
		frequency int, metadata map[string]string) (bool, error)
	UpdateConfig(setName string, transformer string, freeze bool,
		metadata map[string]string) error
}

func writeFofnSetIfConfigured(writers map[string]fofnDirWriter,
	writerFactory fofnDirWriterFactory, fofnDir, setName, transformer string,
	dir *db.Directory, files []string) error {
	if fofnDir == "" || writerFactory == nil {
		return nil
	}

	writer := writers[fofnDir]
	if writer == nil {
		writer = writerFactory(fofnDir)
		writers[fofnDir] = writer
	}

	metadata := map[string]string{
		"requestor": dir.ClaimedBy,
		"review":    time.Unix(dir.ReviewDate, 0).Format(time.DateOnly),
		"remove":    time.Unix(dir.RemoveDate, 0).Format(time.DateOnly),
	}

	wrote, err := writer.Write(setName, transformer, slices.Values(files),
		int(dir.Frequency), metadata) //nolint:gosec
	if err != nil {
		return err
	}

	if wrote {
		return nil
	}

	return updateConfigWhenRequestorChanged(writer, fofnDir, setName, transformer,
		dir, metadata)
}

func updateConfigWhenRequestorChanged(writer fofnDirWriter, fofnDir, setName,
	transformer string, dir *db.Directory, metadata map[string]string) error {
	changed, err := requestorChanged(fofnDir, setName, dir.ClaimedBy)
	if err != nil {
		return err
	}

	if !changed {
		return nil
	}

	return writer.UpdateConfig(setName, transformer, dir.Frequency == 0, metadata)
}

func requestorChanged(fofnDir, setName, requestor string) (bool, error) {
	config, err := fofn.ReadConfig(filepath.Join(fofnDir, ibackup.SafeName(setName)))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, err
	}

	return config.Metadata["requestor"] != requestor, nil
}

type fofnDirWriterFactory func(baseDir string) fofnDirWriter

type dirRules struct {
	*db.Directory
	Rules   map[string]*db.Rule
	RuleIDs map[int64]*db.Rule
}
