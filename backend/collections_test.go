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
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/db"
	lconfig "github.com/wtsi-hgi/backup-plans/internal/config"
	"github.com/wtsi-hgi/backup-plans/rules"
)

// TODO: Check somehow that doing all these things to collections when applied to a dir correctly updates the rule summaries
// either in here or the rules tests or something
func TestCollections(t *testing.T) {
	Convey("With a configured backend", t, func() {
		u := userHandler(root)

		s := New(newEmptyRoot(t), u.getUser, lconfig.NewConfig(t, nil, nil, nil, 0, nil))

		treeDBPath := createTestTree(t)

		_, err := s.rootDir.AddTree(treeDBPath)
		So(err, ShouldBeNil)

		Convey("You can create collections and retrieve their data", func() {
			code, resp := getResponse(s.Collections, "/api/collections", nil)
			So(code, ShouldEqual, http.StatusOK)

			collections := decodeCollections(t, resp)

			So(collections, ShouldResemble, map[int64]*rules.ColRules{})

			code, resp = getResponse(s.CreateCollection, "/api/collections/create?name=Test&description=testdescription", nil)
			checkNoContent(t, code, resp)

			code, resp = getResponse(s.CreateCollection, "/api/collections/create?name=Test2&description=testdescription", nil)
			checkNoContent(t, code, resp)

			code, resp = getResponse(s.CreateCollection, "/api/collections/create?name=Test&description=testdescription", nil)
			checkErrorResponse(t, code, resp, ErrNameExists)

			code, resp = getResponse(s.CreateCollection, "/api/collections/create?name=&description=testdescription", nil)
			checkErrorResponse(t, code, resp, ErrNoName)

			code, resp = getResponse(s.Collections, "/api/collections", nil)
			So(code, ShouldEqual, http.StatusOK)

			collections = decodeCollections(t, resp)

			So(removeTimesFromCollection(t, collections), ShouldResemble, map[int64]*rules.ColRules{
				1: {
					Collection: &db.Collection{
						Name:        "Test",
						Description: "testdescription",
					},
					Rules: map[string]*db.CollectionRule{},
				},
				2: {
					Collection: &db.Collection{
						Name:        "Test2",
						Description: "testdescription",
					},
					Rules: map[string]*db.CollectionRule{},
				},
			})

			Convey("You can update collection information", func() {
				code, resp = getResponse(
					s.UpdateCollection,
					"/api/collections/update?id=1&name=Test2&description=testdescription2",
					nil,
				)
				checkErrorResponse(t, code, resp, ErrNameExists)

				code, resp = getResponse(
					s.UpdateCollection,
					"/api/collections/update?id=1&name=Test3&description=testdescription3",
					nil,
				)
				So(code, ShouldEqual, http.StatusTeapot)
				So(resp, ShouldEqual, "")

				code, resp = getResponse(s.Collections, "/api/collections", nil)
				So(code, ShouldEqual, http.StatusOK)

				collections = decodeCollections(t, resp)

				So(removeTimesFromCollection(t, collections), ShouldResemble, map[int64]*rules.ColRules{
					1: {
						Collection: &db.Collection{
							Name:        "Test3",
							Description: "testdescription3",
						},
						Rules: map[string]*db.CollectionRule{},
					},
					2: {
						Collection: &db.Collection{
							Name:        "Test2",
							Description: "testdescription",
						},
						Rules: map[string]*db.CollectionRule{},
					},
				})
			})

			Convey("You can create collection rules", func() {
				code, resp = getResponse(
					s.CreateCollectionRule,
					"/api/collections/rules/create?id=1&match=*.txt&action=backup",
					nil,
				)
				checkNoContent(t, code, resp)

				code, resp = getResponse(
					s.CreateCollectionRule,
					"/api/collections/rules/create?id=2&match=*.txt&action=backup",
					nil,
				)
				checkNoContent(t, code, resp)

				code, resp = getResponse(
					s.CreateCollectionRule,
					"/api/collections/rules/create?id=1&match=*.cram&action=manualunchecked&metadata=testmeta",
					nil,
				)
				checkNoContent(t, code, resp)

				code, resp = getResponse(
					s.CreateCollectionRule,
					"/api/collections/rules/create?id=1&match=*.txt&action=nobackup",
					nil,
				)
				checkErrorResponse(t, code, resp, ErrRuleExists)

				code, resp = getResponse(s.Collections, "/api/collections", nil)
				So(code, ShouldEqual, http.StatusOK)

				collections = decodeCollections(t, resp)

				So(removeTimesFromCollection(t, collections), ShouldResemble, map[int64]*rules.ColRules{
					1: {
						Collection: &db.Collection{
							Name:        "Test",
							Description: "testdescription",
						},
						Rules: map[string]*db.CollectionRule{
							"*.txt": {
								CollectionID: 1,
								BackupType:   db.BackupIBackup,
								Metadata:     "",
								Match:        "*.txt",
								Override:     false,
							},
							"*.cram": {
								CollectionID: 1,
								BackupType:   db.BackupManualUnchecked,
								Metadata:     "testmeta",
								Match:        "*.cram",
								Override:     false,
							},
						},
					},
					2: {
						Collection: &db.Collection{
							Name:        "Test2",
							Description: "testdescription",
						},
						Rules: map[string]*db.CollectionRule{
							"*.txt": {
								CollectionID: 2,
								BackupType:   db.BackupIBackup,
								Metadata:     "",
								Match:        "*.txt",
								Override:     false,
							},
						},
					},
				})

				Convey("And update them", func() {
					code, resp = getResponse(
						s.UpdateCollectionRule,
						"/api/collections/rules/update?id=1&action=nobackup",
						nil,
					)
					checkErrorResponse(t, code, resp, ErrInvalidMatch)

					code, resp = getResponse(
						s.UpdateCollectionRule,
						"/api/collections/rules/update?id=1234&action=nobackup&match=*",
						nil,
					)
					checkErrorResponse(t, code, resp, ErrCollectionNotFound)

					code, resp = getResponse(
						s.UpdateCollectionRule,
						"/api/collections/rules/update?id=1&action=nobackup&match=*.txt",
						nil,
					)
					checkNoContent(t, code, resp)

					code, resp = getResponse(s.Collections, "/api/collections", nil)
					So(code, ShouldEqual, http.StatusOK)

					collections = decodeCollections(t, resp)

					So(removeTimesFromCollection(t, collections), ShouldResemble, map[int64]*rules.ColRules{
						1: {
							Collection: &db.Collection{
								Name:        "Test",
								Description: "testdescription",
							},
							Rules: map[string]*db.CollectionRule{
								"*.txt": {
									CollectionID: 1,
									BackupType:   db.BackupNone,
									Metadata:     "",
									Match:        "*.txt",
									Override:     false,
								},
								"*.cram": {
									CollectionID: 1,
									BackupType:   db.BackupManualUnchecked,
									Metadata:     "testmeta",
									Match:        "*.cram",
									Override:     false,
								},
							},
						},
						2: {
							Collection: &db.Collection{
								Name:        "Test2",
								Description: "testdescription",
							},
							Rules: map[string]*db.CollectionRule{
								"*.txt": {
									CollectionID: 2,
									BackupType:   db.BackupIBackup,
									Metadata:     "",
									Match:        "*.txt",
									Override:     false,
								},
							},
						},
					})
				})

				Convey("And delete them", func() {
					code, resp = getResponse(
						s.ClaimDir,
						"/api/dir/claim?dir=/some/path/MyDir/",
						nil,
					)
					So(code, ShouldEqual, http.StatusOK)
					So(resp, ShouldEqual, "\""+root+"\"\n")

					code, resp = getResponse(
						s.CreateRule,
						"/api/rules/create?dir=/some/path/MyDir/&match=Test&isCollection=true",
						nil,
					)
					checkNoContent(t, code, resp)

					code, resp = getResponse(
						s.DeleteCollection,
						"/api/collections/delete?id=1",
						nil,
					)
					checkErrorResponse(t, code, resp, ErrCollectionInUse)

					code, resp = getResponse(
						s.RemoveRules,
						"/api/rules/remove?dir=/some/path/MyDir/&match=Test&isCollection=true",
						nil,
					)
					checkNoContent(t, code, resp)

					code, resp = getResponse(
						s.DeleteCollection,
						"/api/collections/delete?id=1",
						nil,
					)
					checkNoContent(t, code, resp)

					code, resp = getResponse(s.Collections, "/api/collections", nil)
					So(code, ShouldEqual, http.StatusOK)

					collections = decodeCollections(t, resp)

					So(removeTimesFromCollection(t, collections), ShouldResemble, map[int64]*rules.ColRules{
						2: {
							Collection: &db.Collection{
								Name:        "Test2",
								Description: "testdescription",
							},
							Rules: map[string]*db.CollectionRule{
								"*.txt": {
									CollectionID: 2,
									BackupType:   db.BackupIBackup,
									Metadata:     "",
									Match:        "*.txt",
									Override:     false,
								},
							},
						},
					})

					code, resp = getResponse(
						s.DeleteCollectionRule,
						"/api/collections/rules/delete?id=1&match=*",
						nil,
					)
					checkErrorResponse(t, code, resp, ErrRuleNotFound)

					code, resp = getResponse(s.Collections, "/api/collections", nil)
					So(code, ShouldEqual, http.StatusOK)

					collections = decodeCollections(t, resp)

					So(removeTimesFromCollection(t, collections), ShouldResemble, map[int64]*rules.ColRules{
						2: {
							Collection: &db.Collection{
								Name:        "Test2",
								Description: "testdescription",
							},
							Rules: map[string]*db.CollectionRule{
								"*.txt": {
									CollectionID: 2,
									BackupType:   db.BackupIBackup,
									Metadata:     "",
									Match:        "*.txt",
									Override:     false,
								},
							},
						},
					})

					code, resp = getResponse(
						s.DeleteCollectionRule,
						"/api/collections/rules/delete?id=1&match=*.txt",
						nil,
					)
					checkNoContent(t, code, resp)

					code, resp = getResponse(s.Collections, "/api/collections", nil)
					So(code, ShouldEqual, http.StatusOK)

					collections = decodeCollections(t, resp)

					So(removeTimesFromCollection(t, collections), ShouldResemble, map[int64]*rules.ColRules{
						2: {
							Collection: &db.Collection{
								Name:        "Test2",
								Description: "testdescription",
							},
							Rules: map[string]*db.CollectionRule{},
						},
					})
				})
			})

			// Convey("You can delete collections and collection rules", func() {
			// 	code, resp = getResponse(s.CreateCollectionRule, "/api/collections/rules/create?match=*.txt&isCollection=true&action=ibackup", nil)
			// 	checkNoContent(t, code, resp)

			// 	code, resp = getResponse(s.DeleteCollection, "/api/collections/delete?id=1", nil)
			// 	checkErrorResponse(t, code, resp, ErrCollectionInUse)

			// 	code, resp = getResponse(s.DeleteCollectionRule, "/api/collections/rules/delete?id=1", nil)
			// 	checkNoContent(t, code, resp)

			// 	code, resp = getResponse(s.DeleteCollection, "/api/collections/delete?id=1", nil)
			// 	checkNoContent(t, code, resp)

			// 	code, resp = getResponse(s.DeleteCollection, "/api/collections/delete?id=1", nil)
			// 	checkErrorResponse(t, code, resp, ErrCollectionNotFound)
			// })

			// Convey("You can add collections to directories", func() {
			// 	code, resp = getResponse(
			// 		s.CreateRule,
			// 		"/api/rules/create?dir=/some/path/MyDir/&match=Test2&isCollection=true",
			// 		nil,
			// 	)
			// 	checkNoContent(t, code, resp)

			// 	code, resp = getResponse(
			// 		s.CreateRule,
			// 		"/api/rules/create?dir=/some/path/MyDir/&match=Test2&isCollection=true",
			// 		nil,
			// 	)
			// 	checkErrorResponse(t, code, resp, ErrRuleExists)

			// 	u = root

			// 	code, resp = getResponse(
			// 		s.Tree,
			// 		"/api/tree?dir=/some/path/MyDir/",
			// 		nil,
			// 	)
			// 	So(code, ShouldEqual, http.StatusOK)
			// 	So(resp, ShouldNotBeNil)
			// })
		})
	})
}

func removeTimesFromCollection(t *testing.T, colRules map[int64]*rules.ColRules) map[int64]*rules.ColRules {
	t.Helper()

	output := make(map[int64]*rules.ColRules)

	for k, c := range colRules {
		So(c.Created, ShouldBeGreaterThan, 0)
		So(c.Modified, ShouldBeGreaterThan, 0)

		c.Created = 0
		c.Modified = 0

		for _, r := range c.Rules {
			So(r.Created, ShouldBeGreaterThan, 0)
			So(r.Modified, ShouldBeGreaterThan, 0)

			r.Created = 0
			r.Modified = 0
		}

		output[k] = c
	}

	return output
}

func removeTimesFromCollectionRules(t *testing.T, rules []db.CollectionRule) []db.CollectionRule {
	t.Helper()

	output := make([]db.CollectionRule, 0, len(rules))

	for k, r := range rules {
		So(r.Created, ShouldBeGreaterThan, 0)
		So(r.Modified, ShouldBeGreaterThan, 0)

		r.Created = 0
		r.Modified = 0

		output[k] = r
	}

	return output
}

func decodeCollections(t *testing.T, resp string) map[int64]*rules.ColRules {
	t.Helper()

	collections := make(map[int64]*rules.ColRules)
	So(json.NewDecoder(strings.NewReader(resp)).Decode(&collections), ShouldBeNil)

	return collections
}

// add tests for applying collections to directories
// check it correctly updates the tree/file size/count/unplanned number calculations/dirsummaries including cache
// add test for adding rule to collection and check it updates numbers accordingly

// sort out auto-apply collection modifications to other dirs (toggle) how to
