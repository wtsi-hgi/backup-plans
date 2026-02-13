/*******************************************************************************
 * Copyright (c) 2026 Genome Research Ltd.
 *
 * Author: Sky Haines <sh55@sanger.ac.uk>
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
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/internal/config"
	"github.com/wtsi-hgi/backup-plans/internal/plandb"
	"vimagination.zapto.org/tree"
)

// NOTE: This test could do with being expanded to evaluate a larger set of test data.
func TestUserGroups(t *testing.T) {
	Convey("With a configured backend", t, func() {
		var u userHandler

		testDB, _ := plandb.PopulateExamplePlanDB(t)
		tr := plandb.ExampleTree()

		treeFile := filepath.Join(t.TempDir(), "tree.db")
		f, err := os.Create(treeFile)
		So(err, ShouldBeNil)

		So(tree.Serialise(f, tr), ShouldBeNil)
		So(f.Close(), ShouldBeNil)

		s, err := New(testDB, u.getUser, config.NewConfig(t, nil, nil, nil, 0))
		So(err, ShouldBeNil)

		So(s.AddTree(treeFile), ShouldBeNil)

		Convey("You can call getUserGroups to retrieve a collection of user, BOM and group information", func() {
			code, resp := getResponse(s.UserGroups, "/api/usergroups", nil)
			So(code, ShouldEqual, http.StatusOK)

			var usergroups = userGroupsBOM{}

			err = json.NewDecoder(strings.NewReader(resp)).Decode(&usergroups)
			So(err, ShouldBeNil)

			So(usergroups, ShouldResemble, userGroupsBOM{
				Users: []string{
					"daemon",
					"bin",
				},
				Groups: []string{
					"daemon",
					"bin",
				},
				Owners: nil,
				BOM:    nil,
			})
		})
	})
}
