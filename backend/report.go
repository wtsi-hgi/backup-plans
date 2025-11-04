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
	"strings"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	"github.com/wtsi-hgi/backup-plans/ruletree"
	"vimagination.zapto.org/tree"
)

type summary struct {
	Summaries    map[string]*ruletree.DirSummary
	Rules        map[uint64]*db.Rule
	Directories  map[string][]uint64
	BackupStatus map[string]*ibackup.SetBackupActivity
}

// Summary is an HTTP endpoint that produces a backup summary of all the
// directories that were passed as reporting roots to the New function.
func (s *Server) Summary(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.summary)
}

func (s *Server) summary(w http.ResponseWriter, _ *http.Request) error { //nolint:funlen,cyclop,gocognit,gocyclo
	dirSummary := summary{
		Summaries:    make(map[string]*ruletree.DirSummary, len(s.reportRoots)),
		Rules:        make(map[uint64]*db.Rule),
		Directories:  make(map[string][]uint64),
		BackupStatus: make(map[string]*ibackup.SetBackupActivity),
	}

	dirClaims := make(map[string]string)

	s.rulesMu.RLock()

	for _, root := range s.reportRoots {
		ds, err := s.rootDir.Summary(root[1:])
		if errors.Is(err, ruletree.ErrNotFound) || errors.As(err, new(tree.ChildNotFoundError)) {
			continue
		} else if err != nil {
			s.rulesMu.RUnlock()

			return err
		}

		clear(ds.Children)

		for _, dir := range s.dirs {
			if strings.HasPrefix(dir.Path, root) && dir.Path != root {
				child, err := s.rootDir.Summary(dir.Path[1:])
				if err != nil {
					continue
				}

				clear(child.Children)

				child.ClaimedBy = s.getClaimed(dir.Path)
				ds.Children[dir.Path] = child
			}
		}

		ds.ClaimedBy = s.getClaimed(root)

		dirSummary.Summaries[root] = ds

		for _, rule := range ds.RuleSummaries {
			if _, ok := dirSummary.Rules[rule.ID]; ok {
				continue
			}

			if rule.ID > 0 {
				dir := s.directoryRules[s.dirs[uint64(s.rules[rule.ID].DirID())].Path] //nolint:gosec

				dirClaims[dir.Path] = dir.ClaimedBy
				if _, ok := dirSummary.Directories[dir.Path]; !ok {
					var ruleIDs []uint64

					for _, r := range dir.Rules {
						ruleIDs = append(ruleIDs, uint64(r.ID())) //nolint:gosec
					}

					slices.Sort(ruleIDs)

					dirSummary.Directories[dir.Path] = ruleIDs
				}
			}

			dirSummary.Rules[rule.ID] = s.rules[rule.ID]
		}
	}

	s.rulesMu.RUnlock()

	for dir, claimedBy := range dirClaims {
		sba, err := s.cache.GetBackupActivity("plan::"+dir, claimedBy)
		if err != nil && err.Error() != "set with that id does not exist" {
			return err
		}

		dirSummary.BackupStatus[dir] = sba
	}

	w.Header().Set("Content-type", "application/json")

	return json.NewEncoder(w).Encode(dirSummary)
}

func (s *Server) getClaimed(root string) string {
	dirRules, ok := s.directoryRules[root]
	if ok {
		return dirRules.ClaimedBy
	}

	return ""
}
