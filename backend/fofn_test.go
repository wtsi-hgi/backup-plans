/*******************************************************************************
 * Copyright (c) 2025 Genome Research Ltd.
 *
 * Author: Michael Woolnough <mw31@sanger.ac.uk>
 *         Sky Haines <sh55@sanger.ac.uk>
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
	"os/user"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/internal/config"
	"github.com/wtsi-hgi/backup-plans/internal/testdb"
	"github.com/wtsi-hgi/backup-plans/internal/testirods"
)

func TestFofn(t *testing.T) {
	Convey("With a configured backend", t, func() {
		var u userHandler

		second, err := user.LookupId("2")
		So(err, ShouldBeNil)

		user, err := user.Current()
		So(err, ShouldBeNil)

		So(testirods.AddPseudoIRODsToolsToPathIfRequired(t), ShouldBeNil)

		s, err := New(testdb.CreateTestDatabase(t), u.getUser, config.NewConfig(t, nil, nil, nil, 0))
		So(err, ShouldBeNil)

		treeDBPath := createTestTree(t)

		So(s.AddTree(treeDBPath), ShouldBeNil)

		Convey("You cannot upload a fofn", func() {
			code, resp := getResponse(s.Fofn, "/test", strings.NewReader("[]"))
			So(code, ShouldEqual, http.StatusBadRequest)
			So(resp, ShouldEqual, "invalid action\n")

			code, resp = getResponse(s.Fofn, "/test?action=backup", strings.NewReader("["))
			So(code, ShouldEqual, http.StatusBadRequest)
			So(resp, ShouldEqual, "unexpected EOF\n")

			code, resp = getResponse(s.Fofn, "/test?action=backup", strings.NewReader("[]"))
			So(code, ShouldEqual, http.StatusBadRequest)
			So(resp, ShouldEqual, "invalid dir path\n")

			code, resp = getResponse(
				s.Fofn,
				"/test?action=backup&dir=/some/path/MyDir/",
				strings.NewReader(`[]`),
			)
			So(resp, ShouldEqual, "invalid user\n")
			So(code, ShouldEqual, http.StatusForbidden)

			u = userHandler(user.Username)

			code, resp = getResponse(
				s.Fofn,
				"/test?action=backup&dir=/some/path/MyDir/",
				strings.NewReader(`["/some/path/MyDir/a.txt","/some/path/MyDir/a.txt"]`),
			)
			So(code, ShouldEqual, http.StatusBadRequest)
			So(resp, ShouldEqual, "unable to add duplicate: /some/path/MyDir/a.txt\n")

			code, resp = getResponse(
				s.Fofn,
				"/test?action=backup&dir=/some/path/MyDir/",
				strings.NewReader(`["/some/pa/MyDir/a.txt","/some/path/MyDir/a.txt"]`),
			)
			So(code, ShouldEqual, http.StatusBadRequest)
			So(resp, ShouldEqual, "invalid filepath: /some/pa/MyDir/a.txt\n")

			code, resp = getResponse(s.ClaimDir, "/test?dir=/some/path/MyDir/", nil)
			So(code, ShouldEqual, http.StatusOK)
			So(resp, ShouldEqual, "\""+user.Username+"\"\n")

			u = "root"

			code, resp = getResponse(
				s.Fofn,
				"/test?action=backup&dir=/some/path/MyDir/",
				strings.NewReader(`["/some/path/MyDir/a.txt"]`),
			)
			So(code, ShouldEqual, http.StatusNotAcceptable)
			So(resp, ShouldEqual, "directory already claimed\n")

			u = userHandler(user.Username)

			code, resp = getResponse(
				s.Fofn,
				"/test?action=backup&dir=/some/path/YourDir/",
				strings.NewReader(`["/some/path/YourDir/a.txt"]`),
			)
			So(code, ShouldEqual, http.StatusNotAcceptable)
			So(resp, ShouldEqual, "cannot claim directory\n")

			code, resp = getResponse(
				s.CreateRule,
				"/test?action=backup&&match=a.txt&dir=/some/path/MyDir/",
				nil,
			)
			So(resp, ShouldEqual, "")
			So(code, ShouldEqual, http.StatusNoContent)
		})

		Convey("You can upload a fofn to add rules", func() {
			u = userHandler(user.Username)
			code, resp := getResponse(
				s.Fofn,
				"/test?action=backup&dir=/some/path/MyDir/",
				strings.NewReader(`["/some/path/MyDir/a.txt","/some/path/MyDir/b.csv","/some/path/MyDir/c.txt"]`),
			)
			So(resp, ShouldEqual, "")
			So(code, ShouldEqual, http.StatusNoContent)

			code, resp = getResponse(
				s.Fofn,
				"/test?action=backup&dir=/some/path/ChildDir/",
				strings.NewReader(`["/some/path/ChildDir/a.txt","/some/path/ChildDir/Child/a.file"]`),
			)
			So(resp, ShouldEqual, "")
			So(code, ShouldEqual, http.StatusNoContent)

			// Check tree correctly updates
			re := regexp.MustCompile("[0-9]{5,}")

			code, resp = getResponse(s.Tree, "/?dir=/some/path/ChildDir/", nil)
			So(code, ShouldEqual, http.StatusOK)
			So(re.ReplaceAllString(resp, "0"), ShouldEqual, "{\"RuleSummaries\":[{\"ID\":4,\"Users\":[{\"Name\":\""+user.Username+"\",\"MTime\":36,\"Files\":1,\"Size\":35}],\"Groups\":[{\"Name\":\""+second.Username+"\",\"MTime\":36,\"Files\":1,\"Size\":35}]},{\"ID\":5,\"Users\":[{\"Name\":\""+user.Username+"\",\"MTime\":36,\"Files\":1,\"Size\":35}],\"Groups\":[{\"Name\":\""+second.Username+"\",\"MTime\":36,\"Files\":1,\"Size\":35}]}],\"Children\":{\"Child/\":{\"ClaimedBy\":\"\",\"RuleSummaries\":[{\"ID\":4,\"Users\":[{\"Name\":\""+user.Username+"\",\"MTime\":36,\"Files\":1,\"Size\":35}],\"Groups\":[{\"Name\":\""+second.Username+"\",\"MTime\":36,\"Files\":1,\"Size\":35}]}],\"Children\":{}}},\"ClaimedBy\":\""+user.Username+"\",\"Rules\":{\"/some/path/ChildDir/\":{\"5\":{\"BackupType\":1,\"Metadata\":\"\",\"Match\":\"a.txt\",\"Override\":false,\"Created\":0,\"Modified\":0}},\"/some/path/ChildDir/Child/\":{\"4\":{\"BackupType\":1,\"Metadata\":\"\",\"Match\":\"a.file\",\"Override\":false,\"Created\":0,\"Modified\":0}}},\"Unauthorised\":[],\"CanClaim\":true,\"Frequency\":7,\"ReviewDate\":0,\"RemoveDate\":0}\n") //nolint:lll

			Convey("You can upload a fofn to update rules: ", func() {
				code, resp = getResponse(
					s.Fofn,
					"/test?action=backup&dir=/some/path/MyDir/",
					strings.NewReader(`["/some/path/MyDir/a.txt"]`),
				)
				So(resp, ShouldEqual, "")
				So(code, ShouldEqual, http.StatusNoContent)

				// Check tree correctly updates
				code, resp = getResponse(s.Tree, "/?dir=/some/path/MyDir/", nil)
				So(code, ShouldEqual, http.StatusOK)
				So(re.ReplaceAllString(resp, "0"), ShouldEqual, `{"RuleSummaries":[{"ID":1,"Users":[{"Name":"root","MTime":4,"Files":1,"Size":3}],"Groups":[{"Name":"`+second.Username+`","MTime":4,"Files":1,"Size":3}]},{"ID":2,"Users":[{"Name":"`+user.Username+`","MTime":6,"Files":1,"Size":5}],"Groups":[{"Name":"`+second.Username+`","MTime":6,"Files":1,"Size":5}]}],"Children":{"ChildToClaim/":{"ClaimedBy":"","RuleSummaries":[],"Children":{}},"ChildToNotClaim/":{"ClaimedBy":"","RuleSummaries":[],"Children":{}}},"ClaimedBy":"`+user.Username+`","Rules":{"/some/path/MyDir/":{"1":{"BackupType":1,"Metadata":"","Match":"a.txt","Override":false,"Created":0,"Modified":0},"2":{"BackupType":1,"Metadata":"","Match":"b.csv","Override":false,"Created":0,"Modified":0},"3":{"BackupType":1,"Metadata":"","Match":"c.txt","Override":false,"Created":0,"Modified":0}}},"Unauthorised":[],"CanClaim":true,"Frequency":7,"ReviewDate":0,"RemoveDate":0}`+"\n") //nolint:lll
			})
		})

		Convey("Given a parent dir with set dir details", func() {
			u = "root"

			now := strconv.FormatInt(time.Now().Unix(), 10)
			future := strconv.FormatInt(time.Now().AddDate(0, 1, 0).Unix(), 10)

			code, resp := getResponse(s.ClaimDir, "/test?dir=/some/path/MyDir/", nil)
			So(code, ShouldEqual, http.StatusOK)
			So(resp, ShouldEqual, "\"root\"\n")

			code, resp = getResponse(s.ClaimDir, "/test?dir=/some/path/MyDir/ChildToClaim/", nil)
			So(code, ShouldEqual, http.StatusOK)
			So(resp, ShouldEqual, "\"root\"\n")

			code, resp = getResponse(
				s.SetDirDetails,
				"/api/dir/setDirDetails?dir=/some/path/MyDir/&frequency=10&review="+now+"&remove="+future,
				nil,
			)

			So(resp, ShouldEqual, "")
			So(code, ShouldEqual, http.StatusNoContent)

			code, resp = getResponse(
				s.Tree,
				"/api/tree?dir=/some/path/MyDir/",
				nil,
			)
			So(resp, ShouldContainSubstring, "\"Frequency\":10,\"ReviewDate\":"+now+",\"RemoveDate\":"+future)
			So(code, ShouldEqual, http.StatusOK)

			code, resp = getResponse(
				s.SetDirDetails,
				"/api/dir/setDirDetails?dir=/some/path/MyDir/ChildToClaim/&frequency=50&review="+now+"&remove="+future,
				nil,
			)

			So(resp, ShouldEqual, "")
			So(code, ShouldEqual, http.StatusNoContent)

			code, resp = getResponse(
				s.Tree,
				"/api/tree?dir=/some/path/MyDir/ChildToClaim/",
				nil,
			)
			So(resp, ShouldContainSubstring, "\"Frequency\":50,\"ReviewDate\":"+now+",\"RemoveDate\":"+future)
			So(code, ShouldEqual, http.StatusOK)

			Convey("You can upload a fofn to add rules to child dirs, inheriting directory details", func() {
				code, resp = getResponse(
					s.Fofn,
					"/test?action=backup&dir=/some/path/MyDir/",
					strings.NewReader(`["/some/path/MyDir/ChildToClaim/file1.txt","/some/path/MyDir/ChildToNotClaim/file2.txt"]`),
				)
				So(resp, ShouldEqual, "")
				So(code, ShouldEqual, http.StatusNoContent)

				code, resp = getResponse(
					s.Tree,
					"/api/tree?dir=/some/path/MyDir/ChildToClaim/",
					nil,
				)

				So(resp, ShouldContainSubstring, "\"Frequency\":10,\"ReviewDate\":"+now+",\"RemoveDate\":"+future)
				So(code, ShouldEqual, http.StatusOK)

				code, resp = getResponse(
					s.Tree,
					"/api/tree?dir=/some/path/MyDir/ChildToNotClaim/",
					nil,
				)

				So(resp, ShouldContainSubstring, "\"Frequency\":10,\"ReviewDate\":"+now+",\"RemoveDate\":"+future)
				So(resp, ShouldContainSubstring, "\"ClaimedBy\":\"root\"")
				So(code, ShouldEqual, http.StatusOK)
			})
		})
	})
}
