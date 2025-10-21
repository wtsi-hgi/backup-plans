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
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestUsername(t *testing.T) {
	Convey("You can get a username from a user ID", t, func() {
		So(Username(uint32(os.Getuid())), ShouldNotBeBlank)

		Convey("The cache is used for a username that's already been fetched", func() {
			const (
				fakeID       = 1234567
				fakeUsername = "MY_REAL_USERNAME"
			)

			userCache.Set(fakeID, fakeUsername)

			So(Username(fakeID), ShouldEqual, fakeUsername)
		})
	})
}

func TestGroup(t *testing.T) {
	Convey("You can get a group name from a group ID", t, func() {
		So(Group(uint32(os.Getgid())), ShouldNotBeBlank)

		Convey("The cache is used for a group name that's already been fetched", func() {
			const (
				fakeID    = 1234567
				fakeGroup = "MY_REAL_GROUP"
			)

			groupCache.Set(fakeID, fakeGroup)

			So(Group(fakeID), ShouldEqual, fakeGroup)
		})
	})
}
