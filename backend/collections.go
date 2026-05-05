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

package backend

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/wtsi-hgi/backup-plans/db"
)

// Collection is an HTTP endpoint that returns all collections.
func (s *Server) Collections(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.collection)
}

func (s *Server) collection(w http.ResponseWriter, _ *http.Request) error {
	w.Header().Set("Content-type", "application/json")

	return json.NewEncoder(w).Encode(s.rootDir.GetCollections())
}

// CreateCollection is an HTTP endpoint that creates a new collection with the given name and description.
func (s *Server) CreateCollection(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.createCollection)
}

func (s *Server) createCollection(w http.ResponseWriter, r *http.Request) error {
	name := r.FormValue("name")
	description := r.FormValue("description")

	if name == "" {
		return ErrNoName
	}

	w.Header().Set("Content-type", "application/json")

	return s.rootDir.CreateCollection(db.Collection{Name: name, Description: description})
}

// UpdateCollection is an HTTP endpoint that updates the name and/or description of a collection.
// Fields that are not provided will not be updated.
// Collection names must be unique.
func (s *Server) UpdateCollection(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.updateCollection)
}

func (s *Server) updateCollection(w http.ResponseWriter, r *http.Request) error {
	id := r.FormValue("id")
	name := r.FormValue("name")
	description := r.FormValue("description")

	cID, err := strconv.ParseInt(id, 10, 0)
	if err != nil {
		return ErrInvalidID
	}

	w.WriteHeader(http.StatusTeapot)

	return s.rootDir.UpdateCollection(cID, name, description)
}

// DeleteCollection is an HTTP endpoint that deletes a collection by ID. A collection cannot be
// deleted if it is applied to one or more directories.
func (s *Server) DeleteCollection(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.deleteCollection)
}

func (s *Server) deleteCollection(w http.ResponseWriter, r *http.Request) error {
	id := r.FormValue("id")

	cID, err := strconv.ParseInt(id, 10, 0)
	if err != nil {
		return ErrInvalidID
	}
	// TODO: Should potentially also check the user should be allowed to here
	return s.rootDir.DeleteCollection(cID)
}

func (s *Server) RemoveCollectionFromDir(w http.ResponseWriter, r *http.Request) {}

// CreateCollectionRule is an HTTP endpoint that creates a new collection rule
// and adds it to the given collection (specified via collection name)
func (s *Server) CreateCollectionRule(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.createCollectionRule)
}

func (s *Server) createCollectionRule(w http.ResponseWriter, r *http.Request) error {
	name := r.FormValue("name")

	rules, err := GetRuleDetails(r)
	if err != nil {
		return err
	}

	var collectionRules []*db.CollectionRule

	for _, rule := range rules {
		collectionRules = append(collectionRules, &db.CollectionRule{
			BackupType: rule.BackupType,
			Match:      rule.Match,
			Metadata:   rule.Metadata,
			Override:   rule.Override,
		})
	}

	if len(collectionRules) == 0 {
		return ErrNoRule
	}

	return s.rootDir.CreateCollectionRules(name, collectionRules)
}
func (s *Server) GetCollectionRules(w http.ResponseWriter, r *http.Request)   {}
func (s *Server) UpdateCollectionRule(w http.ResponseWriter, r *http.Request) {}
func (s *Server) DeleteCollectionRule(w http.ResponseWriter, r *http.Request) {}
