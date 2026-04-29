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
	"errors"
	"fmt"
	"net/http"

	"github.com/wtsi-hgi/backup-plans/db"
)

var (
	ErrNoName = Error{
		Code: http.StatusBadRequest,
		Err:  errors.New("no name provided"), //nolint:err113
	}
	ErrNameExists = Error{
		Code: http.StatusBadRequest,
		Err:  errors.New("collection with that name already exists"), //nolint:err113
	}
)

// Collection is an HTTP endpoint that returns all collections.
func (s *Server) Collections(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.collection)
}

func (s *Server) collection(w http.ResponseWriter, _ *http.Request) error {
	w.Header().Set("Content-type", "application/json")

	return json.NewEncoder(w).Encode(s.collections)
}

// CreateCollection is an HTTP endpoint that creates a new collection with the given name and description.
func (s *Server) CreateCollection(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.createCollection)
}

func (s *Server) createCollection(w http.ResponseWriter, r *http.Request) error {
	fmt.Println("Create collection called :))))")

	name := r.FormValue("name")
	description := r.FormValue("description")

	if name == "" {
		return ErrNoName
	}

	if _, exists := s.collectionNames[name]; exists {

		return ErrNameExists
	}

	c := &db.Collection{
		Name:        name,
		Description: description,
	}

	// TODO: Locking after cache rebase
	err := s.rulesDB.CreateCollection(c)
	if err != nil {
		return err
	}

	s.rulesMu.Lock()
	defer s.rulesMu.Unlock()

	s.collections[c.ID()] = c
	s.collectionNames[c.Name] = c.ID()

	w.WriteHeader(http.StatusNoContent)

	return err
}

// since a rule that links a collection stores its name, this is not definately unique, so how do i get the collection from that?
// map on the server?
func (s *Server) UpdateCollection(w http.ResponseWriter, r *http.Request) {}

// A collection should not be allowed to be removed if it is applied to any directory (rules has a rule with isCollection=True and match=collectionName)
func (s *Server) DeleteCollection(w http.ResponseWriter, r *http.Request) {}

func (s *Server) CreateCollectionRule(w http.ResponseWriter, r *http.Request) {}

func (s *Server) UpdateCollectionRule(w http.ResponseWriter, r *http.Request) {}
func (s *Server) DeleteCollectionRule(w http.ResponseWriter, r *http.Request) {}
