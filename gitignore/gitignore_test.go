package gitignore

import (
	_ "embed"
	"path"
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/wrstat-ui/summary"
	"github.com/wtsi-hgi/wrstat-ui/summary/group"
)

//go:embed gitignoreExample.txt
var exampleData string

func TestNew(t *testing.T) {
	Convey("Given gitIgnore data, you can make rules from it", t, func() {
		layout := "02-01-2006"
		review, err := time.Parse(layout, "01-10-2025")
		So(err, ShouldBeNil)

		remove, err := time.Parse(layout, "01-01-2026")
		So(err, ShouldBeNil)

		config := Config{
			BackupType: db.BackupIBackup,
			Frequency:  7,
			Metadata:   "example-metadata",
			ReviewDate: review,
			RemoveDate: remove,
		}

		rules, err := ToRules(strings.NewReader(exampleData), config)

		So(err, ShouldBeNil)
		So(rules, ShouldNotBeNil)

		expected := db.Rule{
			BackupType: db.BackupNone,
			Metadata:   config.Metadata,
			ReviewDate: review,
			RemoveDate: remove,
			Frequency:  7,
			Match:      "*.log"}
		So(rules[1], ShouldResemble, expected)

		expected.Match = "/build/*"
		So(rules[2], ShouldResemble, expected)

		expected.BackupType = db.BackupIBackup
		expected.Match = "/important.log"
		So(rules[3], ShouldResemble, expected)

		expected.Match = "*"
		So(rules[0], ShouldResemble, expected)

		Convey("A statemachine can be created", func() {
			dir := "/test/dir"

			var ruleList []group.PathGroup[db.Rule]

			for _, r := range rules {
				ruleList = append(ruleList, group.PathGroup[db.Rule]{
					Path:  []byte(path.Join(dir, r.Match)),
					Group: &r,
				})
			}

			sm, err := group.NewStatemachine(ruleList)
			So(err, ShouldBeNil)
			So(sm, ShouldNotBeNil)

			root := &summary.DirectoryPath{Name: "/", Depth: 0}
			rootChild := &summary.DirectoryPath{Name: "test/", Depth: 1, Parent: root}
			testDir := &summary.DirectoryPath{Name: "dir/", Depth: 2, Parent: rootChild}

			matchingRule := sm.GetGroup(&summary.FileInfo{
				Path: testDir,
				Name: []byte("important.log"),
			})
			So(matchingRule, ShouldEqual, &rules[3])

			matchingRule = sm.GetGroup(&summary.FileInfo{
				Path: testDir,
				Name: []byte("other.log"),
			})
			So(matchingRule, ShouldEqual, &rules[1])

			buildDir := &summary.DirectoryPath{Name: "build/", Depth: 3, Parent: testDir}
			matchingRule = sm.GetGroup(&summary.FileInfo{
				Path: buildDir,
				Name: []byte("file.txt"),
			})
			So(matchingRule, ShouldEqual, &rules[2])

			matchingRule = sm.GetGroup(&summary.FileInfo{
				Path: testDir,
				Name: []byte("file.txt"),
			})
			So(matchingRule, ShouldEqual, &rules[0])

		})

		Convey("A gitignore file can be exported", func() {
			exportedData, err := FromRules(rules)
			So(err, ShouldBeNil)
			So(exportedData, ShouldEqual, "!*\n*.log\n/build/*\n!/important.log")
		})
	})
}
