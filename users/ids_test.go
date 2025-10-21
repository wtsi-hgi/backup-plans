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

package users

import (
	"os/user"
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestIDs(t *testing.T) {
	Convey("You can get user and groups IDs from a username", t, func() {
		me, err := user.Current()
		So(err, ShouldBeNil)

		uid, gids := GetIDs(me.Username)
		So(strconv.FormatUint(uint64(uid), 10), ShouldEqual, me.Uid)
		So(gids, ShouldNotBeNil)

		Convey("The cache is used for a username that's already been fetched", func() {
			const (
				fakeID       = 1234567
				fakeUsername = "MY_REAL_USERNAME"
			)

			fakeGroups := []uint32{4, 8, 1, 5, 16, 23, 42}

			userGroupsCache.Set(fakeUsername, groups{
				expiry: time.Now().Add(time.Hour),
				uid:    fakeID,
				groups: fakeGroups,
			})

			uid, gids := GetIDs(fakeUsername)
			So(uid, ShouldEqual, fakeID)
			So(gids, ShouldResemble, fakeGroups)
		})
	})
}
