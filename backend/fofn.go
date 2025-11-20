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

// Fofn takes a FOFN in the request body and adds a corresponding rule for each
// valid path.
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

	var dirDetails dirDetails

	if dr, ok := s.directoryRules[dir]; ok {
		dirDetails.Frequency = dr.Frequency
		dirDetails.ReviewDate = dr.ReviewDate
		dirDetails.RemoveDate = dr.RemoveDate
	} else {
		dirDetails = defaultDirDetails()
	}

	rulesToAdd, err := s.createRulesToAdd(user, rule, files, dirDetails)
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

		if err := s.validateClaimAndRule(user, dir, file, uid, groups); err != nil {
			return err
		}
	}

	return nil
}

func (s *Server) validateClaimAndRule(user, dir, file string, uid uint32, groups []uint32) error {
	fileDir, _ := splitDir(file)

	if !strings.HasPrefix(file, dir) {
		return Error{
			Code: http.StatusBadRequest,
			Err:  fmt.Errorf("invalid filepath: %s", file), //nolint:err113
		}
	}

	if got := s.directoryRules[fileDir]; got != nil { //nolint:nestif
		if got.ClaimedBy != user {
			return ErrDirectoryClaimed
		}
	} else if !s.canClaim(fileDir, uid, groups) {
		return ErrCannotClaimDirectory
	}

	return nil
}

// splitDir splits a file path into directory and match.
// It splits at the last '/' before any '*' character.
func splitDir(file string) (string, string) {
	i := strings.Index(file, "*")
	if i == -1 {
		i = len(file)
	}

	i = strings.LastIndex(file[:i], "/")

	return file[:i+1], file[i+1:]
}

func (s *Server) createRulesToAdd(user string, rule db.Rule, files []string,
	dirDetails dirDetails) ([]ruletree.DirRule, error) {
	rulesToAdd := make([]ruletree.DirRule, 0, len(files))

	detailsSet := make(map[*ruletree.DirRules]bool)

	// add rules
	for _, file := range files {
		// claim dir
		dirRules, err := s.claimAndCreateDirRules(file, user, dirDetails, detailsSet)
		if err != nil {
			return nil, err
		}

		// add rule
		add, err := s.createRuleToAdd(file, rule, dirRules)
		if err != nil {
			return nil, err
		}

		if add != nil {
			rulesToAdd = append(rulesToAdd, *add)
		}
	}

	return rulesToAdd, nil
}

func (s *Server) claimAndCreateDirRules(file, user string, dirDetails dirDetails,
	detailsSet map[*ruletree.DirRules]bool) (*ruletree.DirRules, error) {
	fileDir, _ := splitDir(file)
	dirRules := s.directoryRules[fileDir]

	if dirRules == nil {
		if err := s.claimDirectory(fileDir, user, dirDetails); err != nil {
			return nil, err
		}

		dirRules = s.directoryRules[fileDir]

		return dirRules, nil
	}

	if !detailsSet[dirRules] {
		detailsSet[dirRules] = true

		dirRules.Frequency = dirDetails.Frequency
		dirRules.ReviewDate = dirDetails.ReviewDate
		dirRules.RemoveDate = dirDetails.RemoveDate

		if err := s.rulesDB.UpdateDirectory(dirRules.Directory); err != nil {
			return nil, err
		}
	}

	return dirRules, nil
}

func (s *Server) createRuleToAdd(file string, rule db.Rule, dirRules *ruletree.DirRules) (*ruletree.DirRule, error) {
	newRule := rule
	_, newRule.Match = splitDir(file)

	if existingRule, ok := dirRules.Rules[newRule.Match]; ok {
		if err := s.updateRuleTo(existingRule, &newRule); err != nil {
			return nil, err
		}

		return nil, nil //nolint:nilnil
	}

	if err := s.addRuleToDir(dirRules, &newRule); err != nil {
		return nil, err
	}

	// build rules slice
	add := ruletree.DirRule{
		Directory: dirRules.Directory,
		Rule:      &newRule,
	}

	return &add, nil
}

// GetDirectories takes an array of paths, and returns a same ordered array of
// booleans indicating whether each path points to a directory.
func (s *Server) GetDirectories(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.getDirectories)
}

func (s *Server) getDirectories(w http.ResponseWriter, r *http.Request) error {
	var paths []string

	err := json.NewDecoder(r.Body).Decode(&paths)
	if err != nil {
		return err
	}

	dirs := make([]bool, len(paths))

	for i, path := range paths {
		dirs[i] = s.rootDir.IsDirectory(path)
	}

	w.Header().Set("Content-type", "application/json")

	return json.NewEncoder(w).Encode(dirs)
}
