package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ruletree"
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

func (s *Server) ClaimDir(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.claimDir)
}

func (s *Server) claimDir(w http.ResponseWriter, r *http.Request) error {
	user := s.getUser(r)
	if user == "" {
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

func (s *Server) PassDirClaim(w http.ResponseWriter, r *http.Request) {
}

func (s *Server) RevokeDirClaim(w http.ResponseWriter, r *http.Request) {
}

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
	}

	if rule.ReviewDate = parseTime(r.FormValue("review")); rule.ReviewDate.IsZero() {
		return nil, ErrInvalidTime
	}

	if rule.RemoveDate = parseTime(r.FormValue("remove")); rule.RemoveDate.IsZero() {
		return nil, ErrInvalidTime
	}

	return rule, nil
}

func parseTime(str string) time.Time {
	unix, err := strconv.ParseUint(str, 10, 64)
	if err != nil || unix <= 0 {
		return time.Time{}
	}

	return time.Unix(int64(unix), 0)
}

func (s *Server) GetRules(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.getRules)
}

func (s *Server) getRules(w http.ResponseWriter, r *http.Request) error {
	dir, err := getDir(r)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/json")

	s.rulesMu.RLock()
	defer s.rulesMu.RUnlock()

	directory, ok := s.directoryRules[dir]
	if ok {
		return json.NewEncoder(w).Encode(directory)

	}

	w.Write([]byte{'{', '}'})

	return nil
}

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
	ErrInvalidTime = Error{
		Code: http.StatusBadRequest,
		Err:  errors.New("invalid time"),
	}
	ErrNoRule = Error{
		Code: http.StatusBadRequest,
		Err:  errors.New("no matching rule"),
	}
)
