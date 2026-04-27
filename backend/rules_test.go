/*******************************************************************************
 * Copyright (c) 2025 Genome Research Ltd.
 *
 * Author: Michael Woolnough <mw31@sanger.ac.uk>
 *
 * Permission is hereby granted, free of charge, to any person obtaining
 * a copy of this software and associated documentation files (the
 * "Software"), to deal in the Software without restriction, including
 * without limitation the rights to use, copy, modify, merge, publish,
 * distribute, sublicense, and/or sell copies of the Software, and to
 * permit persons to whom the Software is furnished to do so, subject to
 * the following conditions:
 *
 * The above copyright notice and this permission notice shall be included
 * in all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
 * EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
 * MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
 * IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
 * CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
 * TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
 * SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 ******************************************************************************/

package backend

import (
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"testing"
	"testing/synctest"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/config"
	lconfig "github.com/wtsi-hgi/backup-plans/internal/config"
	"github.com/wtsi-hgi/backup-plans/internal/directories"
	"github.com/wtsi-hgi/backup-plans/internal/plandb"
	"github.com/wtsi-hgi/backup-plans/internal/testdb"
	"github.com/wtsi-hgi/backup-plans/internal/testirods"
	"github.com/wtsi-hgi/backup-plans/users"
	"github.com/wtsi-hgi/ibackup/fofn"
	"github.com/wtsi-hgi/ibackup/set"
	"vimagination.zapto.org/tree"
)

type userHandler string

func (u *userHandler) getUser(_ *http.Request) string {
	return string(*u)
}

const root = "root"

func TestClaimDir(t *testing.T) {
	Convey("With a configured backend", t, func() {
		var u userHandler

		user, err := user.Current()
		So(err, ShouldBeNil)

		s, err := New(testdb.CreateTestDatabase(t), u.getUser, lconfig.NewConfig(t, nil, nil, nil, 0, nil))
		So(err, ShouldBeNil)

		treeDBPath := createTestTree(t)

		So(s.AddTree(treeDBPath), ShouldBeNil)

		Convey("You can claim directories", func() {
			code, resp := getResponse(s.ClaimDir, "/api/dir/claim?dir=/does/not/exist", nil)
			checkErrorResponse(t, code, resp, ErrInvalidUser)

			u = root

			code, resp = getResponse(s.ClaimDir, "/api/dir/claim?dir=/does/not/exist", nil)
			checkErrorResponse(t, code, resp, ErrInvalidDir)

			code, resp = getResponse(s.ClaimDir, "/api/dir/claim?dir=/some/path/MyDir/", nil)
			So(code, ShouldEqual, http.StatusOK)
			So(resp, ShouldEqual, "\""+root+"\"\n")

			code, resp = getResponse(s.ClaimDir, "/api/dir/claim?dir=/some/path/MyDir/", nil)
			checkErrorResponse(t, code, resp, ErrDirectoryClaimed)

			Convey("You can revoke a claim", func() {
				u = ""

				code, resp = getResponse(s.RevokeDirClaim, "/api/dir/revoke?dir=/does/not/exist", nil)
				checkErrorResponse(t, code, resp, ErrInvalidDir)

				code, resp = getResponse(s.RevokeDirClaim, "/api/dir/revoke?dir=/some/path/MyDir/", nil)
				checkErrorResponse(t, code, resp, ErrInvalidUser)

				u = root

				code, resp = getResponse(s.RevokeDirClaim, "/api/dir/revoke?dir=/some/path/MyDir/", nil)
				So(code, ShouldEqual, http.StatusNoContent)
				So(resp, ShouldEqual, "")

				code, resp = getResponse(s.RevokeDirClaim, "/api/dir/revoke?dir=/some/path/MyDir/", nil)
				checkErrorResponse(t, code, resp, ErrDirectoryNotClaimed)
			})

			Convey("You can pass a claim", func() {
				u = ""

				code, resp = getResponse(
					s.PassDirClaim,
					"/api/dir/pass?dir=/does/not/exist&passTo="+user.Username,
					nil,
				)
				checkErrorResponse(t, code, resp, ErrInvalidDir)

				code, resp = getResponse(
					s.PassDirClaim,
					"/api/dir/pass?dir=/some/path/MyDir/&passTo="+user.Username,
					nil,
				)
				checkErrorResponse(t, code, resp, ErrInvalidUser)

				u = root

				code, resp = getResponse(
					s.PassDirClaim,
					"/api/dir/pass?dir=/some/path/MyDir/&passTo=NOT_A_REAL_USER",
					nil,
				)
				checkErrorResponse(t, code, resp, ErrInvalidUser)

				code, resp = getResponse(
					s.PassDirClaim,
					"/api/dir/pass?dir=/some/path/MyDir/&passTo="+user.Username,
					nil,
				)
				So(code, ShouldEqual, http.StatusNoContent)
				So(resp, ShouldEqual, "")

				code, resp = getResponse(
					s.PassDirClaim,
					"/api/dir/pass?dir=/some/path/MyDir/&passTo="+user.Username,
					nil,
				)
				checkErrorResponse(t, code, resp, ErrInvalidUser)
			})

			now := strconv.FormatInt(time.Now().Unix(), 10)
			future := strconv.FormatInt(time.Now().AddDate(0, 1, 0).Unix(), 10)

			Convey("You can set directory details", func() {
				u = ""

				code, resp = getResponse(
					s.SetDirDetails,
					"/api/dir/setDirDetails?dir=/some/path/MyDir/&frequency=10&frozen=false&review="+now+"&remove="+future,
					nil,
				)
				checkErrorResponse(t, code, resp, ErrInvalidUser)

				u = root

				code, resp = getResponse(
					s.SetDirDetails,
					"/api/dir/setDirDetails?dir=/some/path/MyDir/&frequency=10&frozen=false&review="+now+"&remove="+future,
					nil,
				)

				So(code, ShouldEqual, http.StatusNoContent)
				So(resp, ShouldEqual, "")

				code, resp = getResponse(
					s.Tree,
					"/api/tree?dir=/some/path/MyDir/",
					nil,
				)
				So(code, ShouldEqual, http.StatusOK)
				So(resp, ShouldContainSubstring, "\"Frequency\":10,\"Frozen\":false,\"ReviewDate\":"+now+",\"RemoveDate\":"+future)
			})

			Convey("You cannot set invalid directory details", func() {
				code, resp = getResponse(
					s.SetDirDetails,
					"/api/dir/setDirDetails?dir=/some/path/MyDir/&frequency=-1&frozen=false&review="+now+"&remove="+future,
					nil,
				)

				So(code, ShouldEqual, http.StatusBadRequest)
				So(resp, ShouldEqual, "strconv.ParseUint: parsing \"-1\": invalid syntax\n")

				code, resp = getResponse(
					s.SetDirDetails,
					"/api/dir/setDirDetails?dir=/some/path/MyDir/&frequency=10&frozen=false&review=0&remove="+future,
					nil,
				)
				checkErrorResponse(t, code, resp, ErrInvalidTime)

				code, resp = getResponse(
					s.SetDirDetails,
					"/api/dir/setDirDetails?dir=/some/path/MyDir/&frequency=10&frozen=false&review="+future+"&remove="+now,
					nil,
				)
				checkErrorResponse(t, code, resp, ErrInvalidTime)
			})
		})
	})
}

func TestRules(t *testing.T) {
	Convey("With a configured backend", t, func() {
		u := userHandler(root)

		s, err := New(testdb.CreateTestDatabase(t), u.getUser, lconfig.NewConfig(t, nil, nil, nil, 0, nil))
		So(err, ShouldBeNil)

		treeDBPath := createTestTree(t)

		So(s.AddTree(treeDBPath), ShouldBeNil)

		Convey("You can add rules", func() {
			code, resp := getResponse(
				s.CreateRule,
				"/api/rules/create?dir=/some/path/MyDir/&action=backup&match=*.txt&frequency=7&review=100&remove=200",
				nil,
			)
			checkErrorResponse(t, code, resp, ErrInvalidDir)

			code, resp = getResponse(
				s.ClaimDir,
				"/api/dir/claim?dir=/some/path/MyDir/",
				nil,
			)
			So(code, ShouldEqual, http.StatusOK)
			So(resp, ShouldEqual, "\""+root+"\"\n")

			code, resp = getResponse(
				s.CreateRule,
				"/api/rules/create?dir=/some/path/MyDir/&action=backup&match=*.txt&frequency=7&review=100&remove=200",
				nil,
			)
			So(code, ShouldEqual, http.StatusNoContent)
			So(resp, ShouldEqual, "")

			code, resp = getResponse(
				s.CreateRule,
				"/api/rules/create?dir=/some/path/MyDir/&action=backup&match=*.txt&frequency=7&review=100&remove=200",
				nil,
			)
			checkErrorResponse(t, code, resp, ErrRuleExists)

			Convey("And remove them", func() {
				u = "someone"

				code, resp := getResponse(
					s.RemoveRules,
					"/api/rules/remove?dir=/some/path/MyDir/&action=backup&match=*.txt",
					nil,
				)
				checkErrorResponse(t, code, resp, ErrInvalidUser)

				u = root

				code, resp = getResponse(
					s.RemoveRules,
					"/api/rules/remove?dir=/some/path/MyDir/&action=backup&match=*.tsv",
					nil,
				)
				checkErrorResponse(t, code, resp, ErrNoRule)

				code, resp = getResponse(
					s.RemoveRules,
					"/api/rules/remove?dir=/some/path/MyDir/&action=backup&match=*.txt",
					nil,
				)
				So(code, ShouldEqual, http.StatusNoContent)
				So(resp, ShouldEqual, "")

				code, resp = getResponse(
					s.RemoveRules,
					"/api/rules/remove?dir=/some/path/MyDir/&action=backup&match=*.txt",
					nil,
				)
				checkErrorResponse(t, code, resp, ErrNoRule)
			})
		})

		Convey("You can add multiple rules at once", func() {
			code, resp := getResponse(
				s.ClaimDir,
				"/api/dir/claim?dir=/some/path/MyDir/",
				nil,
			)
			So(code, ShouldEqual, http.StatusOK)
			So(resp, ShouldEqual, "\""+root+"\"\n")

			code, resp = getResponse(
				s.CreateRule,
				"/api/rules/create?dir=/some/path/MyDir/&action=backup&match=*.txt&match=*.txt&match=*.jpg&frequency=7&review=100&remove=200", //nolint:lll
				nil,
			)
			So(code, ShouldEqual, http.StatusNoContent)
			So(resp, ShouldEqual, "")

			code, resp = getResponse(
				s.Tree,
				"/api/tree?dir=/some/path/MyDir/",
				nil,
			)
			So(code, ShouldEqual, http.StatusOK)
			So(resp, ShouldContainSubstring, `{"1":{"BackupType":1,"Metadata":"","Match":"*.jpg",`)
			So(resp, ShouldContainSubstring, `,"2":{"BackupType":1,"Metadata":"","Match":"*.txt",`)

			Convey("And remove multiple rules at once", func() {
				code, resp := getResponse(
					s.RemoveRules,
					"/api/rules/remove?dir=/some/path/MyDir/&action=backup&match=*.txt&match=*.jpg",
					nil,
				)
				So(code, ShouldEqual, http.StatusNoContent)
				So(resp, ShouldEqual, "")
			})
		})

		Convey("You can add rules of every type", func() {
			currUser, err := user.Current()
			So(err, ShouldBeNil)

			secondGroup, err := user.LookupGroupId("2")
			So(err, ShouldBeNil)

			code, resp := getResponse(
				s.ClaimDir,
				"/api/dir/claim?dir=/some/path/ChildDir/Child/",
				nil,
			)
			So(code, ShouldEqual, http.StatusOK)
			So(resp, ShouldEqual, "\""+root+"\"\n")

			for n, typ := range [...]string{
				"nobackup", "backup", "manualibackup",
				"manualgit", "manualunchecked", "manualprefect", "manualnfs",
			} {
				Convey(typ, func() {
					code, resp = getResponse(
						s.CreateRule,
						"/api/rules/create?dir=/some/path/ChildDir/Child/&action="+typ+"&match=*&frequency=7&review=100&remove=200",
						nil,
					)
					So(code, ShouldEqual, http.StatusNoContent)
					So(resp, ShouldEqual, "")

					code, resp = getResponse(
						s.Tree,
						"/api/tree?dir=/some/path/ChildDir/Child/",
						nil,
					)
					So(code, ShouldEqual, http.StatusOK)
					So(resp, ShouldStartWith, `{"Group":"root","RuleSummaries":[{"ID":1,"Users":[{"Name":"`+currUser.Username+`","MTime":36,"Files":1,"Size":35}],"Groups":[{"Name":"`+secondGroup.Name+`","MTime":36,"Files":1,"Size":35}]}],"Children":{},"LastMod":36,"ClaimedBy":"root","Rules":{"/some/path/ChildDir/Child/":{"1":{"BackupType":`+strconv.Itoa(n)+`,"Metadata":"","Match":"*","Override":false,"Created":`) //nolint:lll
				})
			}
		})
	})
}

func TestMelt(t *testing.T) {
	t.Setenv("BACKUP_PLANS_CONNECTION_TEST", "")

	synctest.Test(t, func(t *testing.T) {
		Convey("Given a configured server with an ibackup server", t, func() {
			So(testirods.AddPseudoIRODsToolsToPathIfRequired(t), ShouldBeNil)

			u := userHandler("userA")

			testDB, _ := plandb.PopulateExamplePlanDB(t)
			tr := plandb.ExampleTree()

			treeFile := filepath.Join(t.TempDir(), "tree.db")
			f, err := os.Create(treeFile)
			So(err, ShouldBeNil)

			So(tree.Serialise(f, tr), ShouldBeNil)
			So(f.Close(), ShouldBeNil)

			cfg := filepath.Join(t.TempDir(), "config.yaml")
			fofnDir := t.TempDir()

			So(os.WriteFile(cfg, []byte(""+
				`ibackup:
  servers:
    "server":
      fofndir: `+fofnDir+`
  pathtoserver:
    ^/:
      servername: server
      transformer: prefix=/:/
ibackupcacheduration: 3600`,
			), 0600), ShouldBeNil)

			c, err := config.Parse(cfg)
			So(err, ShouldBeNil)

			s, err := New(testDB, u.getUser, c)
			So(err, ShouldBeNil)

			Reset(s.stop)
			Reset(s.config.GetCachedIBackupClient().Stop)
			Reset(s.config.GetIBackupClient().Stop)

			So(s.AddTree(treeFile), ShouldBeNil)

			Convey("You can temporarily thaw a backup set to get it to overwrite existing files", func() {
				now := time.Now().Unix()

				So(s.directoryRules["/lustre/scratch123/humgen/a/b/"].Melt, ShouldEqual, 0)

				code, resp := getResponse(s.SetDirDetails, "/api/setDetails", url.Values{"dir": {"/lustre/scratch123/humgen/a/b/"}, "frequency": {"1"}, "review": {strconv.FormatInt(now+1000, 10)}, "remove": {strconv.FormatInt(now+2000, 10)}, "frozen": {"true"}, "meltToggle": {"true"}}) //nolint:lll
				So(resp, ShouldBeBlank)
				So(code, ShouldEqual, http.StatusNoContent)

				So(s.directoryRules["/lustre/scratch123/humgen/a/b/"].Melt, ShouldBeGreaterThanOrEqualTo, now)

				ns, err := New(testDB, u.getUser, c)
				So(err, ShouldBeNil)
				So(ns.directoryRules["/lustre/scratch123/humgen/a/b/"].Melt, ShouldBeGreaterThanOrEqualTo, now)

				ns.stop()

				fofnPath := filepath.Join(fofnDir, (&set.Set{Requester: "userA", Name: setNamePrefix + "/lustre/scratch123/humgen/a/b/"}).ID()) //nolint:lll

				So(os.MkdirAll(fofnPath, 0700), ShouldBeNil)
				So(fofn.WriteConfig(fofnPath, fofn.SubDirConfig{
					Transformer: "prefix=/:/",
					Freeze:      true,
					Requester:   "userA",
					Name:        setNamePrefix + "/lustre/scratch123/humgen/a/b/",
				}), ShouldBeNil)
				So(os.WriteFile(filepath.Join(fofnPath, "status"), nil, 0600), ShouldBeNil)

				time.Sleep(61 * time.Minute)

				So(s.directoryRules["/lustre/scratch123/humgen/a/b/"].Melt, ShouldEqual, 0)

				ns, err = New(testDB, u.getUser, c)
				So(err, ShouldBeNil)
				So(ns.directoryRules["/lustre/scratch123/humgen/a/b/"].Melt, ShouldEqual, 0)

				ns.stop()
			})
		})
	})
}

func createTestTree(t *testing.T) string {
	t.Helper()

	user, err := user.Current()
	So(err, ShouldBeNil)

	uid, _ := users.GetIDs(user.Username)

	treeDB := directories.NewRoot("/some/path/", time.Now().Unix())
	treeDB.AddDirectory("MyDir").UID = uid
	treeDB.AddDirectory("ChildDir").UID = uid
	treeDB.AddDirectory("MyDir").AddDirectory("ChildToClaim")
	treeDB.AddDirectory("MyDir").AddDirectory("ChildToNotClaim")
	directories.AddFile(&treeDB.Directory, "MyDir/a.txt", 0, 2, 3, 4)
	directories.AddFile(&treeDB.Directory, "MyDir/b.csv", uid, 2, 5, 6)
	directories.AddFile(&treeDB.Directory, "YourDir/c.tsv", 21, 22, 15, 16)
	directories.AddFile(&treeDB.Directory, "OtherDir/a.file", 1, 22, 25, 26)
	directories.AddFile(&treeDB.Directory, "OtherDir/b.file", 1, 2, 35, 36)
	directories.AddFile(&treeDB.Directory, "ChildDir/a.txt", uid, 2, 35, 36)
	directories.AddFile(&treeDB.Directory, "ChildDir/Child/a.file", uid, 2, 35, 36)

	treeDBPath := filepath.Join(t.TempDir(), "a.db")

	f, err := os.Create(treeDBPath)
	So(err, ShouldBeNil)
	So(tree.Serialise(f, treeDB), ShouldBeNil)
	So(f.Close(), ShouldBeNil)

	return treeDBPath
}
