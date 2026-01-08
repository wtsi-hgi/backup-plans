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
	"maps"
	"net/http"
	"slices"
)

type userGroupsBOM struct {
	Users, Groups []string
	Owners, BOM   map[string][]string
}

// UserGroups is an HTTP endpoint that returns the complete list of users and
// groups in the mounted filesystem trees, and maps of Owners->Groups and
// BOM->Groups.
func (s *Server) UserGroups(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.userGroups)
}

func (s *Server) userGroups(w http.ResponseWriter, _ *http.Request) error {
	summary, err := s.rootDir.Summary("")
	if err != nil {
		return err
	}

	users := make(map[string]struct{})
	groups := make(map[string]struct{})

	for _, rule := range summary.RuleSummaries {
		for _, u := range rule.Users {
			users[u.Name] = struct{}{}
		}

		for _, g := range rule.Groups {
			groups[g.Name] = struct{}{}
		}
	}

	owners := s.config.GetOwners()
	bom := s.config.GetBOMs()

	w.Header().Set("Content-Type", "application/json")

	return json.NewEncoder(w).Encode(userGroupsBOM{
		Users:  slices.Collect(maps.Keys(users)),
		Groups: slices.Collect(maps.Keys(groups)),
		Owners: owners, BOM: bom,
	})
}
