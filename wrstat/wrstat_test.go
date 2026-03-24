/*******************************************************************************
 * Copyright (c) 2026 Genome Research Ltd.
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

package wrstat_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/internal/plandb"
	"github.com/wtsi-hgi/backup-plans/internal/wrstat"
)

func TestWRStat(t *testing.T) {
	Convey("With a wrstat database and server and a configured WRStat client", t, func() {
		client, _ := wrstat.NewTestWRStatClient(t, plandb.ExampleTree())

		Convey("You can request mtimes", func() {
			ts, err := client.GetWRStatModTime("/lustre/scratch123/humgen/a/b/")
			So(err, ShouldBeNil)
			So(ts.Unix(), ShouldEqual, 98767)

			ts, err = client.GetWRStatModTime("/lustre/scratch123/humgen//a/b")
			So(err, ShouldBeNil)
			So(ts.Unix(), ShouldEqual, 98767)

			ts, err = client.GetWRStatModTime("/not/a/path/")
			So(err, ShouldNotBeNil)
			So(ts, ShouldBeZeroValue)
		})
	})
}
