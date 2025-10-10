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

package backend

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ruletree"
	"github.com/wtsi-hgi/backup-plans/users"
	"github.com/wtsi-hgi/wrstat-ui/summary/group"
)

func (s *Server) loadRules() ([]ruletree.DirRule, error) {
	s.directoryRules = make(map[string]*ruletree.DirRules)
	s.dirs = make(map[uint64]*db.Directory)
	s.rules = make(map[uint64]*db.Rule)
	dirs := make(map[int64]*ruletree.DirRules)
	dirRules := make([]ruletree.DirRule, 0)

	if err := s.rulesDB.ReadDirectories().ForEach(func(dir *db.Directory) error {
		dr := &ruletree.DirRules{
			Directory: dir,
			Rules:     make(map[string]*db.Rule),
		}
		s.directoryRules[dir.Path] = dr
		dirs[dir.ID()] = dr
		s.dirs[uint64(dir.ID())] = dir

		return nil
	}); err != nil {
		return nil, err
	}

	var ruleList []group.PathGroup[db.Rule]

	if err := s.rulesDB.ReadRules().ForEach(func(r *db.Rule) error {
		dir, ok := dirs[r.DirID()]
		if !ok {
			return ErrOrphanedRule
		}

		s.rules[uint64(r.ID())] = r

		dir.Rules[r.Match] = r
		ruleList = append(ruleList, group.PathGroup[db.Rule]{
			Path:  []byte(dir.Path + r.Match),
			Group: r,
		})

		dirRules = append(dirRules, ruletree.DirRule{
			Directory: dir.Directory,
			Rule:      r,
		})

		return nil
	}); err != nil {
		return nil, err
	}

	sm, err := group.NewStatemachine(ruleList)
	if err != nil {
		return nil, err
	}

	s.stateMachine = sm

	return dirRules, nil
}

// ClaimDir is an HTTP endpoint that allows a user to claim a directory in order
// to add rules to it. The user must be the owner of the directory, in the group
// of the directory, own a file within the directory tree, or be in a group that
// owns a file within the directory tree.
//
// The directory is taken from the 'dir' GET param and the username is
// determined by calling the getUser func passed to New().
func (s *Server) ClaimDir(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.claimDir)
}

func (s *Server) claimDir(w http.ResponseWriter, r *http.Request) error {
	user := s.getUser(r)

	uid, groups := users.GetIDs(s.getUser(r))
	if groups == nil {
		return ErrInvalidUser
	}

	dir, err := getDir(r)
	if err != nil {
		return err
	}

	s.rulesMu.Lock()
	defer s.rulesMu.Unlock()

	if _, ok := s.directoryRules[dir]; ok {
		return ErrDirectoryClaimed
	}

	if !s.canClaim(dir, uid, groups) {
		return ErrCannotClaimDirectory
	}

	directory := &db.Directory{
		Path:      dir,
		ClaimedBy: user,
	}

	if err := s.rulesDB.CreateDirectory(directory); err != nil {
		return err
	}

	s.directoryRules[dir] = &ruletree.DirRules{
		Directory: directory,
		Rules:     make(map[string]*db.Rule),
	}

	s.dirs[uint64(directory.ID())] = directory

	w.Header().Set("Content-Type", "application/json")

	return json.NewEncoder(w).Encode(user)
}

func (s *Server) canClaim(dir string, uid uint32, groups []uint32) bool {
	duid, dgid, err := s.rootDir.GetOwner(dir[1:])
	if err != nil {
		return false
	}

	return uid == duid || slices.Contains(groups, dgid)
}

// PassDirClaim allows the claimant of a directory to pass that claim to another
// user. The other use must satisfy the same conditions as the initial user had
// to in ClaimDir.
//
// Also like in ClaimDir, the directory is taken from the 'dir' GET param. The
// new username is given in the 'passTo' GET param.
func (s *Server) PassDirClaim(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.passDirClaim)
}

func (s *Server) passDirClaim(w http.ResponseWriter, r *http.Request) error {
	user := s.getUser(r)
	passTo := r.FormValue("passTo")

	uid, groups := users.GetIDs(passTo)
	if groups == nil {
		return ErrInvalidUser
	}

	dir, err := getDir(r)
	if err != nil {
		return err
	}

	s.rulesMu.Lock()
	defer s.rulesMu.Unlock()

	directory, ok := s.directoryRules[dir]
	if !ok {
		return ErrDirectoryNotClaimed
	}

	if directory.ClaimedBy != user {
		return ErrInvalidUser
	}

	if !s.canClaim(dir, uid, groups) {
		return ErrCannotClaimDirectory
	}

	directory.ClaimedBy = passTo

	return s.rulesDB.UpdateDirectory(directory.Directory)
}

// RevokeDirClaim allows the claimant of a directory to remove their claim on a
// directory.
//
// This is only allowed on directories without rules.
//
// Like in ClaimDir, the directory is taken from the 'dir' GET param.
func (s *Server) RevokeDirClaim(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.revokeDirClaim)
}

func (s *Server) revokeDirClaim(w http.ResponseWriter, r *http.Request) error {
	user := s.getUser(r)

	dir, err := getDir(r)
	if err != nil {
		return err
	}

	s.rulesMu.Lock()
	defer s.rulesMu.Unlock()

	directory, ok := s.directoryRules[dir]
	if !ok {
		return ErrDirectoryNotClaimed
	}

	if directory.ClaimedBy != user {
		return ErrInvalidUser
	}

	if len(directory.Rules) > 0 {
		return ErrInvalidDir
	}

	delete(s.directoryRules, dir)

	return s.rulesDB.RemoveDirectory(directory.Directory)
}

// CreateRule allows the claimant of a directory to add a rule to that
// directory.
//
// Like in ClaimDir, the directory is taken from the 'dir' GET param.
//
// The following are the GET params for the rule:
//
//	match       The match rule.
//	action      One of nobackup, tempbackup, backup, or manualbackup.
//	metadata    For a manualbackup, it's the requestor of the backup set.
//	frequency   How often in days to run the backup.
//	reviewdate  Unix seconds representing the date of the backup review.
//	removedate  Unix seconds representing the date of the backup removal.
func (s *Server) CreateRule(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.createRule)
}

func (s *Server) createRule(w http.ResponseWriter, r *http.Request) error {
	dir, err := getDir(r)
	if err != nil {
		return err
	}

	rule, err := getRuleDetails(r)
	if err != nil {
		return err
	}

	s.rulesMu.Lock()
	defer s.rulesMu.Unlock()

	directory, ok := s.directoryRules[dir]
	if !ok {
		return ErrInvalidDir
	}

	if directory.ClaimedBy != s.getUser(r) {
		return ErrInvalidUser
	}

	if _, ok := directory.Rules[rule.Match]; ok {
		return ErrRuleExists
	}

	if err := s.rulesDB.CreateDirectoryRule(directory.Directory, rule); err != nil {
		return err
	}

	directory.Rules[rule.Match] = rule
	s.rules[uint64(rule.ID())] = rule

	if err := s.rootDir.AddRule(directory.Directory, rule); err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)

	return nil
}

func getRuleDetails(r *http.Request) (*db.Rule, error) {
	rule := new(db.Rule)

	var requireMetadata, requireFrequency bool

	switch r.FormValue("action") {
	case "nobackup":
		rule.BackupType = db.BackupNone
		requireFrequency = true
	case "tempbackup":
		rule.BackupType = db.BackupTemp
		requireFrequency = true
	case "backup":
		rule.BackupType = db.BackupIBackup
		requireFrequency = true
	case "manualbackup":
		rule.BackupType = db.BackupManual
		requireMetadata = true
	default:
		return nil, ErrInvalidAction
	}

	if requireMetadata {
		rule.Metadata = r.FormValue("metadata")
	}

	if requireFrequency {
		freq, err := strconv.ParseUint(r.FormValue("frequency"), 10, 64)
		if err != nil {
			return nil, ErrInvalidFrequency
		}

		rule.Frequency = uint(freq)
	}

	rule.Match = r.FormValue("match")
	if rule.Match == "" {
		rule.Match = "*"
	} else if strings.Contains(rule.Match, "/") {
		return nil, ErrInvalidMatch
	}

	var err error

	if rule.ReviewDate, err = strconv.ParseInt(r.FormValue("review"), 10, 64); err != nil {
		return nil, ErrInvalidTime
	}

	if rule.RemoveDate, err = strconv.ParseInt(r.FormValue("remove"), 10, 64); err != nil {
		return nil, ErrInvalidTime
	}

	return rule, nil
}

// UpdateRule allows the claimant of a directory to update a rule for that
// directory. The rule is identified by the match string and, as such, cannot be
// changed.
//
// The input matches that of CreateRule.
func (s *Server) UpdateRule(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.updateRule)
}

func (s *Server) updateRule(w http.ResponseWriter, r *http.Request) error {
	dir, err := getDir(r)
	if err != nil {
		return err
	}

	rule, err := getRuleDetails(r)
	if err != nil {
		return err
	}

	s.rulesMu.Lock()
	defer s.rulesMu.Unlock()

	directory, ok := s.directoryRules[dir]
	if !ok {
		return ErrInvalidDir
	}

	if directory.ClaimedBy != s.getUser(r) {
		return ErrInvalidUser
	}

	existingRule, ok := directory.Rules[rule.Match]
	if !ok {
		return ErrNoRule
	}

	existingRule.BackupType = rule.BackupType
	existingRule.Frequency = rule.Frequency
	existingRule.Metadata = rule.Metadata
	existingRule.RemoveDate = rule.RemoveDate
	existingRule.ReviewDate = rule.ReviewDate

	if err := s.rulesDB.UpdateRule(existingRule); err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)

	return nil
}

// RemoveRule allows the claimant of a directory to remove a rule from that
// directory.
//
// Like in ClaimDir, the directory is taken from the 'dir' GET param. The rule
// is determined by the 'match' GET param.
func (s *Server) RemoveRule(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.removeRule)
}

func (s *Server) removeRule(w http.ResponseWriter, r *http.Request) error {
	dir, err := getDir(r)
	if err != nil {
		return err
	}

	s.rulesMu.Lock()
	defer s.rulesMu.Unlock()

	directory, ok := s.directoryRules[dir]
	if !ok {
		return ErrDirectoryNotClaimed
	}

	if directory.ClaimedBy != s.getUser(r) {
		return ErrInvalidUser
	}

	match := r.FormValue("match")
	rule, ok := directory.Rules[match]
	if !ok {
		return ErrNoRule
	}

	if err := s.rulesDB.RemoveRule(rule); err != nil {
		return err
	}

	delete(directory.Rules, match)

	if err := s.rootDir.RemoveRule(directory.Directory, rule); err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)

	return nil
}

func getDir(r *http.Request) (string, error) {
	dir := r.FormValue("dir")

	if !strings.HasPrefix(dir, "/") || !strings.HasSuffix(dir, "/") {
		return "", ErrInvalidDir
	}

	return dir, nil
}

var (
	ErrOrphanedRule = errors.New("rule found without directory")
	ErrInvalidDir   = Error{
		Code: http.StatusBadRequest,
		Err:  errors.New("invalid dir path"),
	}
	ErrInvalidUser = Error{
		Code: http.StatusBadRequest,
		Err:  errors.New("invalid user"),
	}
	ErrDirectoryClaimed = Error{
		Code: http.StatusNotAcceptable,
		Err:  errors.New("directory already claimed"),
	}
	ErrCannotClaimDirectory = Error{
		Code: http.StatusNotAcceptable,
		Err:  errors.New("cannot claim directory"),
	}
	ErrDirectoryNotClaimed = Error{
		Code: http.StatusNotAcceptable,
		Err:  errors.New("directory not claimed"),
	}
	ErrRuleExists = Error{
		Code: http.StatusBadRequest,
		Err:  errors.New("rule already exists for that match string"),
	}
	ErrInvalidFrequency = Error{
		Code: http.StatusBadRequest,
		Err:  errors.New("invalid frequency"),
	}
	ErrInvalidAction = Error{
		Code: http.StatusBadRequest,
		Err:  errors.New("invalid action"),
	}
	ErrInvalidMatch = Error{
		Code: http.StatusBadRequest,
		Err:  errors.New("invalid match string"),
	}
	ErrInvalidTime = Error{
		Code: http.StatusBadRequest,
		Err:  errors.New("invalid time"),
	}
	ErrNoRule = Error{
		Code: http.StatusBadRequest,
		Err:  errors.New("no matching rule"),
	}
)
