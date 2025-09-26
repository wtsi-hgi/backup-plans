package gitignore

import (
	_ "embed"
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/db"
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
		So(rules[0], ShouldResemble, expected)

		expected.Match = "/build/"
		So(rules[1], ShouldResemble, expected)

		expected.BackupType = db.BackupIBackup
		expected.Match = "!/important.log"
		So(rules[2], ShouldResemble, expected)

		// TODO: Confirm the rules work by creating statemachine with them and testing with example paths (ask Michael for help)
		// Convey() {}

		Convey("A statemachine can be created", func() {
			// examplePaths := `
			// test/rules.log
			// test/build/works.txt
			// test/important.log`

		})

		Convey("A gitignore file can be exported", func() {
			exportedData, err := FromRules(rules)
			So(err, ShouldBeNil)
			So(exportedData, ShouldEqual, exampleData)
		})
	})
}
