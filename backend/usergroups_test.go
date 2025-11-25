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
	"os/user"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestOwners(t *testing.T) {
	Convey("A owners file can be correctly parsed in to a map", t, func() {
		u, err := user.Current()
		So(err, ShouldBeNil)

		group, err := user.LookupGroupId(u.Gid)
		So(err, ShouldBeNil)

		second, err := user.LookupGroupId("1")
		So(err, ShouldBeNil)

		m, err := parseOwners(strings.NewReader(``))
		So(err, ShouldBeNil)
		So(m, ShouldBeEmpty)

		m, err = parseOwners(strings.NewReader(u.Gid + ",ownerA"))
		So(err, ShouldBeNil)
		So(m, ShouldResemble, map[string][]string{
			"ownerA": {group.Name},
		})

		m, err = parseOwners(strings.NewReader(u.Gid + ",ownerA\n" + second.Gid + ",ownerA"))
		So(err, ShouldBeNil)
		So(m, ShouldResemble, map[string][]string{
			"ownerA": {group.Name, second.Name},
		})

		m, err = parseOwners(strings.NewReader(u.Gid + ",ownerA\n0,ownerB\n" + second.Gid + ",ownerB"))
		So(err, ShouldBeNil)
		So(m, ShouldResemble, map[string][]string{
			"ownerA": {group.Name},
			"ownerB": {"root", second.Name},
		})
	})
}

func TestBOM(t *testing.T) {
	Convey("A bom file can be correctly parsed in to a map", t, func() {
		m, err := parseBOM(strings.NewReader(``))
		So(err, ShouldBeNil)
		So(m, ShouldBeEmpty)

		m, err = parseBOM(strings.NewReader(`group1,bomA`))
		So(err, ShouldBeNil)
		So(m, ShouldResemble, map[string][]string{
			"bomA": {"group1"},
		})

		m, err = parseBOM(strings.NewReader(`` +
			`group1,bomA
group2,bomA`,
		))
		So(err, ShouldBeNil)
		So(m, ShouldResemble, map[string][]string{
			"bomA": {"group1", "group2"},
		})
		m, err = parseBOM(strings.NewReader(`` +
			`group1,bomA
group2,bomB
group3,bomB`,
		))
		So(err, ShouldBeNil)
		So(m, ShouldResemble, map[string][]string{
			"bomA": {"group1"},
			"bomB": {"group2", "group3"},
		})
	})
}
