/*******************************************************************************
 * Copyright (c) 2025 Genome Research Ltd.
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

package db

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCollections(t *testing.T) {
	Convey("With a test database", t, func() {
		db := createTestDatabase(t)

		Convey("You can create a collection with rules", func() {
			c := &Collection{
				Name:        "Test Collection",
				Description: "A collection of rules for testing.",
			}

			So(db.CreateCollection(c), ShouldBeNil)

			ruleA := &CollectionRule{
				BackupType:   BackupIBackup,
				Match:        "*.jpg",
				collectionID: c.ID(),
			}
			ruleB := &CollectionRule{
				BackupType:   BackupIBackup,
				Match:        "*",
				collectionID: c.ID(),
			}

			c2 := &Collection{
				Name: "Second Collection",
			}

			So(db.CreateCollection(c2), ShouldBeNil)

			ruleC := &CollectionRule{
				BackupType:   BackupNone,
				Match:        "*.txt",
				collectionID: c2.ID(),
			}

			So(db.CreateCollectionRule(c, ruleA), ShouldBeNil)
			So(db.CreateCollectionRule(c, ruleB), ShouldBeNil)
			So(db.CreateCollectionRule(c2, ruleC), ShouldBeNil)

			Convey("…and retrieve them from the DB", func() {
				So(collectIter(t, db.ReadCollections()), ShouldResemble, []*Collection{c, c2})
				So(collectIter(t, db.ReadCollectionRules()), ShouldResemble, []*CollectionRule{ruleA, ruleB, ruleC})
			})

			Convey("…and update them", func() {
				c.Description = "An updated description."

				So(db.UpdateCollection(c), ShouldBeNil)
				So(collectIter(t, db.ReadCollections()), ShouldResemble, []*Collection{c, c2})

				ruleA.Override = true
				So(db.UpdateCollectionRule(ruleA), ShouldBeNil)
				So(collectIter(t, db.ReadCollectionRules()), ShouldResemble, []*CollectionRule{ruleA, ruleB, ruleC})
			})

			Convey("…and remove them", func() {
				So(db.RemoveCollectionRule(ruleA), ShouldBeNil)
				So(collectIter(t, db.ReadCollectionRules()), ShouldResemble, []*CollectionRule{ruleB, ruleC})

				So(db.RemoveCollection(c), ShouldBeNil)
				So(collectIter(t, db.ReadCollections()), ShouldResemble, []*Collection{c2})
				So(collectIter(t, db.ReadCollectionRules()), ShouldResemble, []*CollectionRule{ruleC})

				So(db.RemoveCollection(c2), ShouldBeNil)
				So(collectIter(t, db.ReadCollections()), ShouldResemble, []*Collection{})
				So(collectIter(t, db.ReadCollectionRules()), ShouldResemble, []*CollectionRule{})
			})
		})
	})
}
