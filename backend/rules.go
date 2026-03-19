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
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ruletree"
	"github.com/wtsi-hgi/backup-plans/users"
	"github.com/wtsi-hgi/wrstat-ui/summary/group"
)

var (
	ErrOrphanedRule = errors.New("rule found without directory")
	ErrInvalidDir   = Error{
		Code: http.StatusBadRequest,
		Err:  errors.New("invalid dir path"), //nolint:err113
	}
	ErrInvalidUser = Error{
		Code: http.StatusForbidden,
		Err:  errors.New("invalid user"), //nolint:err113
	}
	ErrDirectoryClaimed = Error{
		Code: http.StatusNotAcceptable,
		Err:  errors.New("directory already claimed"), //nolint:err113
	}
	ErrCannotClaimDirectory = Error{
		Code: http.StatusNotAcceptable,
		Err:  errors.New("cannot claim directory"), //nolint:err113
	}
	ErrDirectoryNotClaimed = Error{
		Code: http.StatusNotAcceptable,
		Err:  errors.New("directory not claimed"), //nolint:err113
	}
	ErrRuleExists = Error{
		Code: http.StatusBadRequest,
		Err:  errors.New("rule already exists for that match string"), //nolint:err113
	}
	ErrInvalidFrequency = Error{
		Code: http.StatusBadRequest,
		Err:  errors.New("invalid frequency"), //nolint:err113
	}
	ErrInvalidAction = Error{
		Code: http.StatusBadRequest,
		Err:  errors.New("invalid action"), //nolint:err113
	}
	ErrInvalidMatch = Error{
		Code: http.StatusBadRequest,
		Err:  errors.New("invalid match string"), //nolint:err113
	}
	ErrInvalidTime = Error{
		Code: http.StatusBadRequest,
		Err:  errors.New("invalid time"), //nolint:err113
	}
	ErrNoRule = Error{
		Code: http.StatusBadRequest,
		Err:  errors.New("no matching rule"), //nolint:err113
	}
	ErrDirectoryNotFrozen = Error{
		Code: http.StatusBadRequest,
		Err:  errors.New("directory not frozen"), //nolint:err113
	}
	ErrAlreadyFrozen = Error{
		Code: http.StatusBadRequest,
		Err:  errors.New("directory already frozen"), //nolint:err113
	}
)

const (
	defaultFrequency = 7
	frequencyLimit   = 100000
	month            = 3600 * 24 * 30
	twoyears         = time.Hour * 24 * 365 * 2
)

func (s *Server) loadRules() ([]ruletree.DirRule, error) { //nolint:funlen
	s.directoryRules = make(map[string]*Directory)
	s.dirs = make(map[uint64]*db.Directory)
	s.rules = make(map[uint64]*db.Rule)
	dirs := make(map[int64]*ruletree.DirRules)
	dirRules := make([]ruletree.DirRule, 0)

	if err := s.rulesDB.ReadDirectories().ForEach(func(dir *db.Directory) error {
		dr := Directory{
			&ruletree.DirRules{
				Directory: dir,
				Rules:     make(map[string]*db.Rule),
			},
			nil,
		}
		s.directoryRules[dir.Path] = &dr
		dirs[dir.ID()] = dr.DirRules
		s.dirs[uint64(dir.ID())] = dir //nolint:gosec

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

		s.rules[uint64(r.ID())] = r //nolint:gosec

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

func (s *Server) claimDir(w http.ResponseWriter, r *http.Request) error { //nolint:funlen
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

	err = s.claimDirectory(
		dir,
		user,
		defaultDirDetails(),
	)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/json")

	return json.NewEncoder(w).Encode(user)
}

func defaultDirDetails() dirDetails {
	reviewDate := time.Now().Add(twoyears).Unix()

	return dirDetails{Frequency: defaultFrequency,
		ReviewDate: reviewDate,
		RemoveDate: reviewDate + month,
	}
}

func (s *Server) claimDirectory(fileDir, user string, dirdetails dirDetails) error {
	directory := &db.Directory{
		Path:       fileDir,
		ClaimedBy:  user,
		Frequency:  dirdetails.Frequency,
		ReviewDate: dirdetails.ReviewDate,
		RemoveDate: dirdetails.RemoveDate,
	}

	if err := s.rulesDB.CreateDirectory(directory); err != nil {
		return err
	}

	dirSummary, err := s.rootDir.Summary(directory.Path)
	if err != nil {
		return err
	}

	s.directoryRules[fileDir] = &Directory{
		DirRules: &ruletree.DirRules{
			Directory: directory,
			Rules:     make(map[string]*db.Rule),
		},
		DirSummary: dirSummary,
	}

	s.dirs[uint64(directory.ID())] = directory //nolint:gosec

	s.addToDirMaps(directory.ID(), dirSummary)

	return nil
}

func (s *Server) canClaim(dir string, uid uint32, groups []uint32) bool {
	duid, dgid, err := s.rootDir.GetOwner(dir[1:])
	if err != nil {
		return false
	}

	return uid == duid || slices.Contains(groups, dgid)
}

// PassDirClaim allows the claimant of a directory to pass that claim to another
// user. The other user must satisfy the same conditions as the initial user had
// to in ClaimDir.
//
// Also like in ClaimDir, the directory is taken from the 'dir' GET param. The
// new username is given in the 'passTo' GET param.
func (s *Server) PassDirClaim(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.passDirClaim)
}

func (s *Server) passDirClaim(_ http.ResponseWriter, r *http.Request) error { //nolint:funlen
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

func (s *Server) revokeDirClaim(_ http.ResponseWriter, r *http.Request) error {
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

func (s *Server) SetDirDetails(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.setDirDetails)
}

func (s *Server) setDirDetails(_ http.ResponseWriter, r *http.Request) error { //nolint:funlen,gocognit,gocyclo
	dir, err := getDir(r)
	if err != nil {
		return err
	}

	dDetails, err := getDirDetails(r)
	if err != nil {
		return err
	}

	if err = validateDirDetails(dDetails); err != nil {
		return err
	}

	user := s.getUser(r)

	s.rulesMu.Lock()
	defer s.rulesMu.Unlock()

	directory, ok := s.directoryRules[dir]
	if !ok {
		return ErrDirectoryNotClaimed
	}

	if directory.ClaimedBy != user {
		return ErrInvalidUser
	}

	directory.Frequency = dDetails.Frequency
	directory.ReviewDate = dDetails.ReviewDate
	directory.RemoveDate = dDetails.RemoveDate
	directory.Frozen = dDetails.Frozen

	if dDetails.ToggleMelt { //nolint:nestif
		if directory.Melt == 0 {
			directory.Melt = time.Now().Unix()
		} else {
			directory.Melt = 0
		}
	} else if !dDetails.Frozen {
		directory.Melt = 0
	}

	return s.rulesDB.UpdateDirectory(directory.Directory)
}

func validateDirDetails(d dirDetails) error {
	if d.Frequency > frequencyLimit {
		return ErrInvalidFrequency
	}

	remove := d.RemoveDate
	review := d.ReviewDate

	if remove < review {
		return ErrInvalidTime
	}

	if review < time.Now().Unix() {
		return ErrInvalidTime
	}

	return nil
}

type dirDetails struct {
	Frequency  uint
	Frozen     bool
	ReviewDate int64
	RemoveDate int64
	Melt       int64 `json:",omitzero"`
	ToggleMelt bool  `json:",omitzero"`
}

func getDirDetails(r *http.Request) (dirDetails, error) { //nolint:gocyclo,funlen
	frequencyStr := r.FormValue("frequency")
	reviewStr := r.FormValue("review")
	removeStr := r.FormValue("remove")
	frozenStr := r.FormValue("frozen")
	toggleMeltStr := r.FormValue("meltToggle")

	frequency, err := strconv.ParseUint(frequencyStr, 10, 64)
	if err != nil {
		return dirDetails{}, Error{Err: err, Code: http.StatusBadRequest}
	}

	if frequency > frequencyLimit {
		return dirDetails{}, ErrInvalidFrequency
	}

	frozen, err := strconv.ParseBool(frozenStr)
	if err != nil {
		return dirDetails{}, Error{Err: err, Code: http.StatusBadRequest}
	}

	var toggleMelt bool

	if frozen {
		toggleMelt, err = strconv.ParseBool(toggleMeltStr)
		if err != nil {
			return dirDetails{}, Error{Err: err, Code: http.StatusBadRequest}
		}
	}

	review, err := strconv.ParseInt(reviewStr, 10, 64)
	if err != nil {
		return dirDetails{}, Error{Err: err, Code: http.StatusBadRequest}
	}

	remove, err := strconv.ParseInt(removeStr, 10, 64)
	if err != nil {
		return dirDetails{}, Error{Err: err, Code: http.StatusBadRequest}
	}

	return dirDetails{
		Frequency: uint(frequency), Frozen: frozen, ToggleMelt: toggleMelt,
		ReviewDate: review, RemoveDate: remove,
	}, nil
}

// CreateRule allows the claimant of a directory to add a rule to that
// directory.
//
// Like in ClaimDir, the directory is taken from the 'dir' GET param.
//
// The following are the GET params for the rule:
//
//	match       The match rule.
//	action      One of nobackup, backup, manualibackup, manualgit, manualprefect
//				or manualunchecked.
//	metadata    For a manualibackup, it's the requestor of the backup set.
func (s *Server) CreateRule(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.createRule)
}

func (s *Server) createRule(w http.ResponseWriter, r *http.Request) error { //nolint:funlen,gocyclo
	dir, err := getDir(r)
	if err != nil {
		return err
	}

	rules, err := getRuleDetails(r)
	if err != nil {
		return err
	}

	s.buildMu.Lock()
	defer s.buildMu.Unlock()

	directory, err := s.checkAddRules(r, dir, rules)
	if err != nil {
		return err
	}

	if err := s.rulesDB.CreateDirectoryRule(directory.Directory, rules...); err != nil {
		return err
	}

	s.rulesMu.Lock()

	for _, rule := range rules {
		directory.Rules[rule.Match] = rule
		s.rules[uint64(rule.ID())] = rule //nolint:gosec
	}

	s.rulesMu.Unlock()

	if err := s.rootDir.AddRules(directory.Directory, rules); err != nil {
		return err
	}

	if err := s.updateDirSummaries(directory.Path); err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)

	return nil
}

func (s *Server) checkAddRules(r *http.Request, dir string, rules []*db.Rule) (*Directory, error) {
	s.rulesMu.RLock()
	defer s.rulesMu.RUnlock()

	directory, ok := s.directoryRules[dir]
	if !ok {
		return nil, ErrInvalidDir
	}

	if directory.ClaimedBy != s.getUser(r) {
		return nil, ErrInvalidUser
	}

	for _, rule := range rules {
		if _, ok := directory.Rules[rule.Match]; ok {
			return nil, ErrRuleExists
		}
	}

	return directory, nil
}

func getRuleDetails(r *http.Request) ([]*db.Rule, error) { //nolint:cyclop,gocyclo,funlen
	var rule db.Rule

	var requireMetadata bool

	switch r.FormValue("action") {
	case "nobackup":
		rule.BackupType = db.BackupNone
	case "backup":
		rule.BackupType = db.BackupIBackup
	case "manualibackup":
		rule.BackupType = db.BackupManualIBackup
		requireMetadata = true
	case "manualgit":
		rule.BackupType = db.BackupManualGit
		requireMetadata = true
	case "manualprefect":
		rule.BackupType = db.BackupManualPrefect
		requireMetadata = true
	case "manualnfs":
		rule.BackupType = db.BackupManualNFS
		requireMetadata = true
	case "manualunchecked":
		rule.BackupType = db.BackupManualUnchecked
		requireMetadata = true
	default:
		return nil, ErrInvalidAction
	}

	if requireMetadata {
		rule.Metadata = r.FormValue("metadata")
	}

	rule.Override = r.FormValue("override") == "true"

	rules, err := createMatchRules(rule, r.Form["match"])
	if err != nil {
		return nil, err
	} else if len(rules) == 0 {
		rule.Match = "*"
		rules = []*db.Rule{&rule}
	}

	return rules, nil
}

func createMatchRules(rule db.Rule, matches []string) ([]*db.Rule, error) {
	ms := make(map[string]struct{})

	for _, match := range matches {
		if match == "" { //nolint:gocritic,nestif
			match = "*"
		} else if strings.Contains(match, "\x00") {
			return nil, ErrInvalidMatch
		} else if strings.HasSuffix(match, "/") {
			match += "*"
		}

		ms[match] = struct{}{}
	}

	rules := make([]*db.Rule, len(ms))
	matches = slices.Collect(maps.Keys(ms))

	slices.Sort(matches)

	for n, match := range matches {
		rules[n] = &db.Rule{
			BackupType: rule.BackupType,
			Metadata:   rule.Metadata,
			Match:      match,
			Override:   rule.Override,
		}
	}

	return rules, nil
}

// UpdateRule allows the claimant of a directory to update a rule for that
// directory. The rule is identified by the match string and, as such, cannot be
// changed.
//
// The input matches that of CreateRule.
func (s *Server) UpdateRule(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.updateRule)
}

func (s *Server) updateRule(w http.ResponseWriter, r *http.Request) error { //nolint:funlen,gocyclo
	dir, err := getDir(r)
	if err != nil {
		return err
	}

	rules, err := getRuleDetails(r)
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

	for n, rule := range rules {
		existingRule, ok := directory.Rules[rule.Match]
		if !ok {
			return ErrNoRule
		}

		existingRule.BackupType = rule.BackupType
		existingRule.Metadata = rule.Metadata
		rules[n] = existingRule
	}

	if err := s.rulesDB.UpdateRule(rules...); err != nil {
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

func (s *Server) removeRule(w http.ResponseWriter, r *http.Request) error { //nolint:funlen
	dir, err := getDir(r)
	if err != nil {
		return err
	}

	s.buildMu.Lock()
	defer s.buildMu.Unlock()

	directory, rule, err := s.getRuleToRemove(r, dir)
	if err != nil {
		return err
	}

	if err := s.rulesDB.RemoveRule(rule); err != nil {
		return err
	}

	if err := s.rootDir.RemoveRule(directory.Directory, rule); err != nil {
		return err
	}

	s.rulesMu.Lock()
	delete(directory.Rules, rule.Match)
	delete(s.rules, uint64(rule.ID())) //nolint:gosec
	s.rulesMu.Unlock()

	if err := s.updateDirSummaries(directory.Path); err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)

	return nil
}

func (s *Server) getRuleToRemove(r *http.Request, dir string) (*Directory, *db.Rule, error) {
	s.rulesMu.RLock()
	defer s.rulesMu.RUnlock()

	directory, ok := s.directoryRules[dir]
	if !ok {
		return nil, nil, ErrDirectoryNotClaimed
	}

	if directory.ClaimedBy != s.getUser(r) {
		return nil, nil, ErrInvalidUser
	}

	match := r.FormValue("match")

	rule, ok := directory.Rules[match]
	if !ok {
		return nil, nil, ErrNoRule
	}

	return directory, rule, nil
}

func getDir(r *http.Request) (string, error) {
	dir := r.FormValue("dir")

	if !strings.HasPrefix(dir, "/") || !strings.HasSuffix(dir, "/") {
		return "", ErrInvalidDir
	}

	return dir, nil
}
