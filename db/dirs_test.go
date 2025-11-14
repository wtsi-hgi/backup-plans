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

package db

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDirs(t *testing.T) {
	Convey("With a test database", t, func() {
		db := createTestDatabase(t)

		Convey("You can add directories to the database", func() {
			dirA := &Directory{
				Path:       "/some/path/",
				ClaimedBy:  "me",
				Frequency:  7,
				ReviewDate: 1,
				RemoveDate: 2,
			}
			dirB := &Directory{
				Path:      "/some/other/path/",
				ClaimedBy: "someone",
			}
			dirC := &Directory{
				Path:      "/some/path/",
				ClaimedBy: "exists",
			}

			So(db.CreateDirectory(dirA), ShouldBeNil)
			So(dirA.id, ShouldEqual, 1)
			So(db.CreateDirectory(dirB), ShouldBeNil)
			So(dirB.id, ShouldEqual, 2)
			So(db.CreateDirectory(dirA), ShouldNotBeNil)
			So(db.CreateDirectory(dirC), ShouldNotBeNil)

			Convey("…and retrieve them from the DB", func() {
				So(collectIter(t, db.ReadDirectories()), ShouldResemble, []*Directory{dirA, dirB})
			})

			Convey("…and update them", func() {
				dirA.ClaimedBy = "someone else"

				So(db.UpdateDirectory(dirA), ShouldBeNil)
				So(collectIter(t, db.ReadDirectories()), ShouldResemble, []*Directory{dirA, dirB})
			})

			Convey("…and remove them", func() {
				So(db.RemoveDirectory(dirA), ShouldBeNil)
				So(collectIter(t, db.ReadDirectories()), ShouldResemble, []*Directory{dirB})
			})
		})
	})
}

func collectIter[T any](t *testing.T, i *IterErr[T]) []T {
	t.Helper()

	var vs []T

	So(i.ForEach(func(item T) error {
		vs = append(vs, item)

		return nil
	}), ShouldBeNil)

	return vs
}
