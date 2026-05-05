/*******************************************************************************
 * Copyright (c) 2025 Genome Research Ltd.
 *
 * Author: Michael Woolnough <mw31@sanger.ac.uk>
 *		   Sky Haines <sh55@sanger.ac.uk>
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
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/rules"
	"github.com/wtsi-hgi/backup-plans/users"
)

const frequencyLimit = 100000

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

	uid, groups := users.GetIDs(user)
	if groups == nil {
		return ErrInvalidUser
	}

	dir, err := getDir(r)
	if err != nil {
		return err
	}

	if !s.canClaim(dir, uid, groups) {
		return ErrCannotClaimDirectory
	}

	if err := s.rootDir.ClaimDirectory(dir, user); err != nil {
		return err
	}

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
// user. The other user must satisfy the same conditions as the initial user had
// to in ClaimDir.
//
// Also like in ClaimDir, the directory is taken from the 'dir' GET param. The
// new username is given in the 'passTo' GET param.
func (s *Server) PassDirClaim(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.passDirClaim)
}

func (s *Server) passDirClaim(_ http.ResponseWriter, r *http.Request) error {
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

	directory := s.rootDir.ClaimedDirectory(dir)
	if directory == nil {
		return ErrDirectoryNotClaimed
	}

	if directory.ClaimedBy != user {
		return ErrInvalidUser
	}

	if !s.canClaim(dir, uid, groups) {
		return ErrCannotClaimDirectory
	}

	return s.rootDir.PassDirectory(dir, passTo)
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

	directory := s.rootDir.ClaimedDirectory(dir)
	if directory == nil {
		return ErrDirectoryNotClaimed
	}

	if directory.ClaimedBy != user {
		return ErrInvalidUser
	}

	if s.rootDir.HasRules(dir) {
		return ErrInvalidDir
	}

	return s.rootDir.RevokeDirectory(dir)
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

	directory := s.rootDir.ClaimedDirectory(dir)
	if directory == nil {
		return ErrDirectoryNotClaimed
	}

	if directory.ClaimedBy != s.getUser(r) {
		return ErrInvalidUser
	}

	dDetails.Path = dir

	if dDetails.ToggleMelt { //nolint:nestif
		if directory.Melt == 0 {
			dDetails.Melt = time.Now().Unix()
		} else {
			dDetails.Melt = 0
		}
	} else if !dDetails.Frozen {
		dDetails.Melt = 0
	}

	return s.rootDir.SetDirDetails(dDetails.Directory)
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
	rules.Directory
	ToggleMelt bool `json:",omitzero"`
}

func getDirDetails(r *http.Request) (dirDetails, error) { //nolint:gocyclo,funlen
	frequencyStr := r.FormValue("frequency")
	reviewStr := r.FormValue("review")
	removeStr := r.FormValue("remove")
	frozenStr := r.FormValue("frozen")
	toggleMeltStr := r.FormValue("meltToggle")

	frequency, err := strconv.ParseUint(frequencyStr, 10, 64)
	if err != nil {
		return dirDetails{}, err
	}

	if frequency > frequencyLimit {
		return dirDetails{}, ErrInvalidFrequency
	}

	frozen, err := strconv.ParseBool(frozenStr)
	if err != nil {
		return dirDetails{}, err
	}

	var toggleMelt bool

	if frozen {
		toggleMelt, err = strconv.ParseBool(toggleMeltStr)
		if err != nil {
			return dirDetails{}, err
		}
	}

	review, err := strconv.ParseInt(reviewStr, 10, 64)
	if err != nil {
		return dirDetails{}, err
	}

	remove, err := strconv.ParseInt(removeStr, 10, 64)
	if err != nil {
		return dirDetails{}, err
	}

	return dirDetails{
		Directory: rules.Directory{
			Frequency: uint(frequency), Frozen: frozen,
			ReviewDate: review, RemoveDate: remove,
		},
		ToggleMelt: toggleMelt,
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

func (s *Server) createRule(_ http.ResponseWriter, r *http.Request) error {
	dir, err := getDir(r)
	if err != nil {
		return err
	}

	rules, err := GetRuleDetails(r)
	if err != nil {
		return err
	}

	if err := s.userClaimedDir(dir, s.getUser(r)); err != nil {
		return err
	}

	return s.rootDir.AddRules(dir, rules)
}

func (s *Server) userClaimedDir(dir, user string) error {
	directory := s.rootDir.ClaimedDirectory(dir)
	if directory == nil {
		return ErrDirectoryNotClaimed
	}

	if directory.ClaimedBy != user {
		return ErrInvalidUser
	}

	return nil
}

func GetRuleDetails(r *http.Request) ([]rules.Rule, error) { //nolint:cyclop,gocyclo,funlen
	var rule rules.Rule

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

	// rule.IsCollection = r.FormValue("iscollection") == "true"

	ruleList, err := createMatchRules(rule, r.Form["match"])
	if err != nil {
		return nil, err
	} else if len(ruleList) == 0 {
		rule.Match = "*"
		ruleList = []rules.Rule{rule}
	}

	return ruleList, nil
}

func createMatchRules(rule rules.Rule, matches []string) ([]rules.Rule, error) {
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

	ruleList := make([]rules.Rule, len(ms))
	matches = slices.Collect(maps.Keys(ms))

	slices.Sort(matches)

	for n, match := range matches {
		ruleList[n] = rules.Rule{
			BackupType: rule.BackupType,
			Metadata:   rule.Metadata,
			Match:      match,
			Override:   rule.Override,
		}
	}

	return ruleList, nil
}

// UpdateRule allows the claimant of a directory to update a rule for that
// directory. The rule is identified by the match string and, as such, cannot be
// changed.
//
// The input matches that of CreateRule.
func (s *Server) UpdateRule(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.updateRule)
}

func (s *Server) updateRule(_ http.ResponseWriter, r *http.Request) error {
	dir, err := getDir(r)
	if err != nil {
		return err
	}

	rules, err := GetRuleDetails(r)
	if err != nil {
		return err
	}

	if err := s.userClaimedDir(dir, s.getUser(r)); err != nil {
		return err
	}

	return s.rootDir.UpdateRule(dir, rules[0])
}

// RemoveRules allows the claimant of a directory to remove one or more rules
// from that directory.
//
// Like in ClaimDir, the directory is taken from the 'dir' GET param. The rules
// are determined by the 'match' GET param.
func (s *Server) RemoveRules(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.removeRules)
}

func (s *Server) removeRules(_ http.ResponseWriter, r *http.Request) error {
	dir, err := getDir(r)
	if err != nil {
		return err
	}

	if err := s.userClaimedDir(dir, s.getUser(r)); err != nil {
		return err
	}

	matches := r.Form["match"]

	for _, match := range matches {
		err := s.rootDir.RemoveRule(dir, match)
		if err != nil {
			return err
		}
	}

	return nil
}

func getDir(r *http.Request) (string, error) {
	dir := r.FormValue("dir")

	if !strings.HasPrefix(dir, "/") || !strings.HasSuffix(dir, "/") {
		return "", ErrInvalidDir
	}

	return dir, nil
}
