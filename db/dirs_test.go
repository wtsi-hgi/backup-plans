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
				Path:      "/some/path/",
				ClaimedBy: "me",
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
