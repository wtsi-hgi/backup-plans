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
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/internal/directories"
	"github.com/wtsi-hgi/backup-plans/internal/ibackup"
	"github.com/wtsi-hgi/backup-plans/internal/testdb"
	"github.com/wtsi-hgi/backup-plans/internal/testirods"
	"github.com/wtsi-hgi/backup-plans/users"
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

		So(testirods.AddPseudoIRODsToolsToPathIfRequired(t), ShouldBeNil)

		s, err := New(testdb.CreateTestDatabase(t), u.getUser, nil, ibackup.NewMultiClient(t), "")
		So(err, ShouldBeNil)

		treeDBPath := createTestTree(t)

		So(s.AddTree(treeDBPath), ShouldBeNil)

		Convey("You can claim directories", func() {
			code, resp := getResponse(s.ClaimDir, "/api/dir/claim?dir=/does/not/exist", nil)
			So(code, ShouldEqual, http.StatusForbidden)
			So(resp, ShouldEqual, "invalid user\n")

			u = root

			code, resp = getResponse(s.ClaimDir, "/api/dir/claim?dir=/does/not/exist", nil)
			So(code, ShouldEqual, http.StatusBadRequest)
			So(resp, ShouldEqual, "invalid dir path\n")

			code, resp = getResponse(s.ClaimDir, "/api/dir/claim?dir=/some/path/MyDir/", nil)
			So(code, ShouldEqual, http.StatusOK)
			So(resp, ShouldEqual, "\""+root+"\"\n")

			code, resp = getResponse(s.ClaimDir, "/api/dir/claim?dir=/some/path/MyDir/", nil)
			So(code, ShouldEqual, http.StatusNotAcceptable)
			So(resp, ShouldEqual, "directory already claimed\n")

			Convey("You can revoke a claim", func() {
				u = ""

				code, resp = getResponse(s.RevokeDirClaim, "/api/dir/revoke?dir=/does/not/exist", nil)
				So(code, ShouldEqual, http.StatusBadRequest)
				So(resp, ShouldEqual, "invalid dir path\n")

				code, resp = getResponse(s.RevokeDirClaim, "/api/dir/revoke?dir=/some/path/MyDir/", nil)
				So(code, ShouldEqual, http.StatusForbidden)
				So(resp, ShouldEqual, "invalid user\n")

				u = root

				code, resp = getResponse(s.RevokeDirClaim, "/api/dir/revoke?dir=/some/path/MyDir/", nil)
				So(code, ShouldEqual, http.StatusNoContent)
				So(resp, ShouldEqual, "")

				code, resp = getResponse(s.RevokeDirClaim, "/api/dir/revoke?dir=/some/path/MyDir/", nil)
				So(code, ShouldEqual, http.StatusNotAcceptable)
				So(resp, ShouldEqual, "directory not claimed\n")
			})

			Convey("You can pass a claim", func() {
				u = ""

				code, resp = getResponse(
					s.PassDirClaim,
					"/api/dir/pass?dir=/does/not/exist&passTo="+user.Username,
					nil,
				)
				So(code, ShouldEqual, http.StatusBadRequest)
				So(resp, ShouldEqual, "invalid dir path\n")

				code, resp = getResponse(
					s.PassDirClaim,
					"/api/dir/pass?dir=/some/path/MyDir/&passTo="+user.Username,
					nil,
				)
				So(code, ShouldEqual, http.StatusForbidden)
				So(resp, ShouldEqual, "invalid user\n")

				u = root

				code, resp = getResponse(
					s.PassDirClaim,
					"/api/dir/pass?dir=/some/path/MyDir/&passTo=NOT_A_REAL_USER",
					nil,
				)
				So(code, ShouldEqual, http.StatusForbidden)
				So(resp, ShouldEqual, "invalid user\n")

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
				So(code, ShouldEqual, http.StatusForbidden)
				So(resp, ShouldEqual, "invalid user\n")
			})

			now := strconv.FormatInt(time.Now().Unix(), 10)
			future := strconv.FormatInt(time.Now().AddDate(0, 1, 0).Unix(), 10)

			Convey("You can set directory details", func() {
				u = ""

				code, resp = getResponse(
					s.SetDirDetails,
					"/api/dir/setDirDetails?dir=/some/path/MyDir/&frequency=10&review="+now+"&remove="+future,
					nil,
				)
				So(resp, ShouldEqual, "invalid user\n")
				So(code, ShouldEqual, http.StatusForbidden)

				u = root

				code, resp = getResponse(
					s.SetDirDetails,
					"/api/dir/setDirDetails?dir=/some/path/MyDir/&frequency=10&review="+now+"&remove="+future,
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
				So(resp, ShouldContainSubstring, "\"Frequency\":10,\"ReviewDate\":"+now+",\"RemoveDate\":"+future)
			})

			Convey("You cannot set invalid directory details", func() {
				code, resp = getResponse(
					s.SetDirDetails,
					"/api/dir/setDirDetails?dir=/some/path/MyDir/&frequency=-1&review="+now+"&remove="+future,
					nil,
				)

				So(code, ShouldEqual, http.StatusBadRequest)
				So(resp, ShouldEqual, "strconv.ParseUint: parsing \"-1\": invalid syntax\n")

				code, resp = getResponse(
					s.SetDirDetails,
					"/api/dir/setDirDetails?dir=/some/path/MyDir/&frequency=10&review=0&remove="+future,
					nil,
				)

				So(code, ShouldEqual, http.StatusBadRequest)
				So(resp, ShouldEqual, "invalid time\n")

				code, resp = getResponse(
					s.SetDirDetails,
					"/api/dir/setDirDetails?dir=/some/path/MyDir/&frequency=10&review="+future+"&remove="+now,
					nil,
				)

				So(code, ShouldEqual, http.StatusBadRequest)
				So(resp, ShouldEqual, "invalid time\n")
			})
		})
	})
}

func TestRules(t *testing.T) {
	Convey("With a configured backend", t, func() {
		u := userHandler(root)

		So(testirods.AddPseudoIRODsToolsToPathIfRequired(t), ShouldBeNil)

		s, err := New(testdb.CreateTestDatabase(t), u.getUser, nil, ibackup.NewMultiClient(t), "")
		So(err, ShouldBeNil)

		treeDBPath := createTestTree(t)

		So(s.AddTree(treeDBPath), ShouldBeNil)

		Convey("You can add rules", func() {
			code, resp := getResponse(
				s.CreateRule,
				"/api/rules/create?dir=/some/path/MyDir/&action=backup&match=*.txt&frequency=7&review=100&remove=200",
				nil,
			)
			So(code, ShouldEqual, http.StatusBadRequest)
			So(resp, ShouldEqual, "invalid dir path\n")

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
			So(code, ShouldEqual, http.StatusBadRequest)
			So(resp, ShouldEqual, "rule already exists for that match string\n")

			Convey("And remove them", func() {
				u = "someone"

				code, resp := getResponse(
					s.RemoveRule,
					"/api/rules/remove?dir=/some/path/MyDir/&action=backup&match=*.txt",
					nil,
				)
				So(code, ShouldEqual, http.StatusForbidden)
				So(resp, ShouldEqual, "invalid user\n")

				u = root

				code, resp = getResponse(
					s.RemoveRule,
					"/api/rules/remove?dir=/some/path/MyDir/&action=backup&match=*.tsv",
					nil,
				)
				So(code, ShouldEqual, http.StatusBadRequest)
				So(resp, ShouldEqual, "no matching rule\n")

				code, resp = getResponse(
					s.RemoveRule,
					"/api/rules/remove?dir=/some/path/MyDir/&action=backup&match=*.txt",
					nil,
				)
				So(code, ShouldEqual, http.StatusNoContent)
				So(resp, ShouldEqual, "")

				code, resp = getResponse(
					s.RemoveRule,
					"/api/rules/remove?dir=/some/path/MyDir/&action=backup&match=*.txt",
					nil,
				)
				So(code, ShouldEqual, http.StatusBadRequest)
				So(resp, ShouldEqual, "no matching rule\n")
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
