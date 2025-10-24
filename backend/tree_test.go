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
	"os/user"
	"regexp"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/internal/testdb"
	"github.com/wtsi-hgi/backup-plans/users"
)

func TestTree(t *testing.T) {
	Convey("With a configured backend", t, func() {
		var u userHandler

		user, err := user.Current()
		So(err, ShouldBeNil)

		s, err := New(testdb.CreateTestDatabase(t), u.getUser, nil)
		So(err, ShouldBeNil)

		treeDBPath := createTestTree(t)

		So(s.AddTree(treeDBPath), ShouldBeNil)

		Convey("You can get tree information for a directory", func() {
			code, resp := getResponse(
				s.Tree,
				"/api/tree?dir=/some/path/MyDir/",
			)
			So(code, ShouldEqual, http.StatusUnauthorized)
			So(resp, ShouldEqual, "not authorised to see this directory\n")

			u = root

			code, resp = getResponse(
				s.Tree,
				"/api/tree?dir=/some/path/MyDir/",
			)
			So(code, ShouldEqual, http.StatusOK)
			So(resp, ShouldEqual, "{\"RuleSummaries\":[{\"ID\":0,\"Users\":["+
				"{\"Name\":\"root\",\"MTime\":4,\"Files\":1,\"Size\":3},"+
				"{\"Name\":\""+user.Username+"\",\"MTime\":6,\"Files\":1,\"Size\":5}],"+
				"\"Groups\":["+
				"{\"Name\":\"daemon\",\"MTime\":6,\"Files\":2,\"Size\":8}]}"+
				"],\"Children\":{},\"ClaimedBy\":\"\",\"Rules\":{},\"Unauthorised\":[],\"CanClaim\":true}\n")

			code, _ = getResponse(s.ClaimDir, "/api/dir/claim?dir=/some/path/MyDir/")
			So(code, ShouldEqual, http.StatusOK)

			code, _ = getResponse(
				s.CreateRule,
				"/api/rules/create?dir=/some/path/MyDir/&action=backup&match=*.txt&frequency=7&review=100&remove=200",
			)
			So(code, ShouldEqual, http.StatusNoContent)

			code, resp = getResponse(
				s.Tree,
				"/api/tree?dir=/some/path/MyDir/",
			)
			So(code, ShouldEqual, http.StatusOK)

			re := regexp.MustCompile(`Created\":[0-9]+,\"Modified\":[0-9]+`)
			resp = re.ReplaceAllString(resp, "Created\":0,\"Modified\":0")

			So(resp, ShouldEqual, "{\"RuleSummaries\":[{\"ID\":0,\"Users\":["+
				"{\"Name\":\""+user.Username+"\",\"MTime\":6,\"Files\":1,\"Size\":5}"+
				"],\"Groups\":["+
				"{\"Name\":\""+users.Username(2)+"\",\"MTime\":6,\"Files\":1,\"Size\":5}]},"+
				"{\"ID\":1,\"Users\":["+
				"{\"Name\":\"root\",\"MTime\":4,\"Files\":1,\"Size\":3}"+
				"],\"Groups\":["+
				"{\"Name\":\""+users.Group(2)+"\",\"MTime\":4,\"Files\":1,\"Size\":3}]}"+
				"],\"Children\":{},\"ClaimedBy\":\"root\",\"Rules\":{"+
				"\"/some/path/MyDir/\":{\"1\":{\"BackupType\":1,\"Metadata\":\"\",\"ReviewDate\":100,"+
				"\"RemoveDate\":200,\"Match\":\"*.txt\",\"Frequency\":7,\"Created\":0,\"Modified\":0}}},"+
				"\"Unauthorised\":[],\"CanClaim\":true}\n")
		})
	})
}
