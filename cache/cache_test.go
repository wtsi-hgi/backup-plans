package cache

import (
	"io"
	"strconv"
	"testing"
	"testing/synctest"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCache(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		Convey("With a cache", t, func() {
			v := 0

			c := New(time.Hour, func(k string) (string, error) {
				v++

				if v == 3 {
					return "", io.EOF
				}

				return k + strconv.Itoa(v), nil
			})

			Reset(c.Stop)

			Convey("You can get non-cached items", func() {
				av, err := c.Get("a")
				So(err, ShouldBeNil)
				So(av, ShouldEqual, "a1")

				bv, err := c.Get("b")
				So(err, ShouldBeNil)
				So(bv, ShouldEqual, "b2")

				cv, err := c.Get("c")
				So(err, ShouldEqual, io.EOF)
				So(cv, ShouldEqual, "")

				time.Sleep(time.Hour + time.Minute)

				v, err := c.Get("a")
				So(err, ShouldBeNil)
				So(v, ShouldNotEqual, av)
				So(v, ShouldStartWith, "a")

				v, err = c.Get("b")
				So(err, ShouldBeNil)
				So(v, ShouldNotEqual, bv)
				So(v, ShouldStartWith, "b")

				v, err = c.Get("c")
				So(err, ShouldBeNil)
				So(v, ShouldStartWith, "c")
			})
		})
	})
}
