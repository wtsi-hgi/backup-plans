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
	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/internal/config"
	"github.com/wtsi-hgi/backup-plans/internal/plandb"
	"vimagination.zapto.org/tree"
)

func TestCollections(t *testing.T) {
	Convey("With a configured backend", t, func() {
		var u userHandler

		testDB, _ := plandb.PopulateExamplePlanDB(t)
		tr := plandb.ExampleTree()

		treeFile := filepath.Join(t.TempDir(), "tree.db")
		f, err := os.Create(treeFile)
		So(err, ShouldBeNil)

		So(tree.Serialise(f, tr), ShouldBeNil)
		So(f.Close(), ShouldBeNil)

		s, err := New(testDB, u.getUser, config.NewConfig(t, nil, nil, nil, 0, nil))
		So(err, ShouldBeNil)

		So(s.AddTree(treeFile), ShouldBeNil)

		Convey("You can retrieve all collections and their rules", func() {
			code, resp := getResponse(s.Collections, "/api/collections", nil)
			So(code, ShouldEqual, http.StatusOK)

			var collections map[string]db.Collection

			err = json.NewDecoder(strings.NewReader(resp)).Decode(&collections)
			So(err, ShouldBeNil)

			So(collections, ShouldResemble, map[string]db.Collection{})

			code, resp = getResponse(s.CreateCollection, "/api/collections/create?name=Test&description=testdescription", nil)
			So(code, ShouldEqual, http.StatusNoContent)
			So(resp, ShouldEqual, "")

			code, resp = getResponse(s.Collections, "/api/collections", nil)
			So(code, ShouldEqual, http.StatusOK)

			err = json.NewDecoder(strings.NewReader(resp)).Decode(&collections)
			So(err, ShouldBeNil)

			So(removeTimes(t, collections), ShouldResemble, map[string]db.Collection{
				"1": {
					Name:        "Test",
					Description: "testdescription",
				},
			})

			Convey("..and update them", func() {

			})

			Convey("..and delete them", func() {

			})
		})
	})
}

func removeTimes(t *testing.T, collections map[string]db.Collection) map[string]db.Collection {
	t.Helper()

	output := make(map[string]db.Collection)

	for k, c := range collections {
		So(c.Created, ShouldBeGreaterThan, 0)
		So(c.Modified, ShouldBeGreaterThan, 0)

		c.Created = 0
		c.Modified = 0

		output[k] = c
	}

	return output
}

// add tests for applying collections to directories
// check it correctly updates the tree/file size/count/unplanned number calculations/dirsummaries including cache
// add test for adding rule to collection and check it updates numbers accordingly

// sort out auto-apply collection modifications to other dirs (toggle) how to
