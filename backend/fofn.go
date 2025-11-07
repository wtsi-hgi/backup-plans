/*******************************************************************************
 * Copyright (c) 2025 Genome Research Ltd.
 *
 * Author: Michael Woolnough <mw31@sanger.ac.uk>
 *         Sky Haines <sh55@sanger.ac.uk>
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
	"path/filepath"
	"slices"
	"strings"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ruletree"
	"github.com/wtsi-hgi/backup-plans/users"
)

const maxFiles = 100

var ErrTooManyFiles = Error{
	Code: http.StatusBadRequest,
	Err:  errors.New("too many files in FOFN"), //nolint:err113
}

// Summary is an HTTP endpoint that produces a backup summary of all the
// directories that were passed as reporting roots to the New function.
func (s *Server) Fofn(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.fofn)
}

func (s *Server) fofn(_ http.ResponseWriter, r *http.Request) error {
	rule, err := getRuleDetails(r)
	if err != nil {
		return err
	}

	var files []string

	err = json.NewDecoder(r.Body).Decode(&files)
	if err != nil {
		return Error{Err: err, Code: http.StatusBadRequest}
	}

	if len(files) > maxFiles {
		return ErrTooManyFiles
	}

	dir, err := getDir(r)
	if err != nil {
		return err
	}

	user := s.getUser(r)

	slices.Sort(files)

	return s.validateAndApplyFofn(user, dir, files, *rule)
}

func (s *Server) validateAndApplyFofn(user, dir string, files []string, rule db.Rule) error {
	s.rulesMu.Lock()
	defer s.rulesMu.Unlock()

	if err := s.validateFofn(user, dir, files); err != nil {
		return err
	}

	rulesToAdd, err := s.createRulesToAdd(user, rule, files)
	if err != nil {
		return err
	}

	return s.rootDir.AddRules(dir, rulesToAdd)
}

func (s *Server) validateFofn(user, dir string, files []string) error {
	uid, groups := users.GetIDs(user)
	if groups == nil {
		return ErrInvalidUser
	}

	prev := ""
	// validate
	for _, file := range files {
		if file == prev {
			return Error{
				Code: http.StatusBadRequest,
				Err:  fmt.Errorf("unable to add duplicate: %s", file), //nolint:err113
			}
		}

		prev = file

		if !strings.HasPrefix(file, dir) {
			return Error{
				Code: http.StatusBadRequest,
				Err:  fmt.Errorf("invalid filepath: %s", file), //nolint:err113
			}
		}

		if err := s.validateClaimAndRule(user, dir, file, uid, groups); err != nil {
			return err
		}
	}

	return nil
}

func (s *Server) validateClaimAndRule(user, dir, file string, uid uint32, groups []uint32) error {
	fileDir := filepath.Dir(file) + "/"

	if got := s.directoryRules[fileDir]; got != nil { //nolint:nestif
		if got.ClaimedBy != user {
			return ErrDirectoryClaimed
		}
	} else if !s.canClaim(dir, uid, groups) {
		return ErrCannotClaimDirectory
	}

	return nil
}

func (s *Server) createRulesToAdd(user string, rule db.Rule, files []string) ([]ruletree.DirRule, error) {
	rulesToAdd := make([]ruletree.DirRule, 0, len(files))

	// add rules
	for _, file := range files {
		// claim dir add
		fileDir := filepath.Dir(file) + "/"
		dirRules := s.directoryRules[fileDir]

		if dirRules == nil {
			if err := s.claimDirectory(fileDir, user); err != nil {
				return nil, err
			}

			dirRules = s.directoryRules[fileDir]
		}

		// add rule
		newRule := rule
		newRule.Match = filepath.Base(file)

		if existingRule, ok := dirRules.Rules[newRule.Match]; ok {
			if err := s.updateRuleTo(existingRule, &newRule); err != nil {
				return nil, err
			}
		} else {
			if err := s.addRuleToDir(dirRules, &newRule); err != nil {
				return nil, err
			}

			// build rules slice
			add := ruletree.DirRule{
				Directory: dirRules.Directory,
				Rule:      &newRule,
			}

			rulesToAdd = append(rulesToAdd, add)
		}
	}

	return rulesToAdd, nil
}

// func (s *Server) RemoveRules(w http.ResponseWriter, r *http.Request) {
// 	handle(w, r, s.removeRule)
// }

// func (s *Server) removeRules(w http.ResponseWriter, r *http.Request) error { //nolint:funlen
// 	var files []string

// 	err := json.NewDecoder(r.Body).Decode(&files)
// 	if err != nil {
// 		return Error{Err: err, Code: http.StatusBadRequest}
// 	}

// 	user := s.getUser(r)

// 	var dirRules map[*db.Directory][]*db.Rule

// 	for _, file := range files {
// 		dir := filepath.Dir(file) + "/"
// 		directory, ok := s.directoryRules[dir]
// 		if !ok {
// 			continue
// 		}

// 		if directory.ClaimedBy == user {

// 		}
// 	}

// 	return nil
// }
