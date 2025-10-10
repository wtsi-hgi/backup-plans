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

package ruletree

import (
	"bytes"
	"cmp"
	"slices"
	"strings"

	"github.com/wtsi-hgi/backup-plans/users"
	"vimagination.zapto.org/byteio"
	"vimagination.zapto.org/tree"
)

// DirSummary contains summarised information about the directory and it's
// children.
type DirSummary struct {
	uid, gid      uint32
	ClaimedBy     string
	RuleSummaries []Rule
	Children      map[string]*DirSummary
}

func (d *DirSummary) mergeRules(rules []Rule) {
	for _, rule := range rules {
		pos, ok := slices.BinarySearchFunc(d.RuleSummaries, rule, func(a, b Rule) int {
			return int(a.ID) - int(b.ID)
		})
		if !ok {
			d.RuleSummaries = slices.Insert(d.RuleSummaries, pos, Rule{ID: rule.ID})
		}

		for _, user := range rule.Users {
			d.RuleSummaries[pos].Users.add(user.id, user.MTime, user.Files, user.Size)
		}

		setNames(d.RuleSummaries[pos].Users, users.Username)

		for _, group := range rule.Groups {
			d.RuleSummaries[pos].Groups.add(group.id, group.MTime, group.Files, group.Size)
		}

		setNames(d.RuleSummaries[pos].Groups, users.Group)
	}
}

// IDs returns the UID and GID for the directory.
func (d *DirSummary) IDs() (uint32, uint32) {
	return d.uid, d.gid
}

func setNames(rules RuleStats, name func(uint32) string) {
	for n := range rules {
		rules[n].Name = name(rules[n].id)
	}
}

type ruleOverlay struct {
	lower, upper *tree.MemTree
}

func (r *ruleOverlay) Summary(path string) (*DirSummary, error) {
	if path == "" {
		return r.getSummaryWithChildren(), nil
	}

	cr, rest, err := r.getChild(path)
	if err != nil {
		return nil, err
	}

	return cr.Summary(rest)
}

func (r *ruleOverlay) getChild(path string) (*ruleOverlay, string, error) {
	pos := strings.IndexByte(path, '/')
	child := path[:pos+1]

	lower, err := r.lower.Child(child)
	if err != nil {
		return nil, "", err
	}

	var upper *tree.MemTree

	if r.upper != nil {
		upper, _ = r.upper.Child(child)
	}

	cr := &ruleOverlay{lower, upper}

	return cr, path[pos+1:], nil
}

func (r *ruleOverlay) GetOwner(path string) (uint32, uint32, error) {
	if path == "" {
		uid, gid := r.getOwner()

		return uid, gid, nil
	}

	cr, rest, err := r.getChild(path)
	if err != nil {
		return 0, 0, err
	}

	return cr.GetOwner(rest)
}

func (r *ruleOverlay) getOwner() (uint32, uint32) {
	sr := byteio.StickyLittleEndianReader{Reader: bytes.NewReader(cmp.Or(r.upper, r.lower).Data())}

	return uint32(sr.ReadUintX()), uint32(sr.ReadUintX())
}

func (r *ruleOverlay) getSummaryWithChildren() *DirSummary {
	ds := r.getSummary()

	for name, lower := range r.lower.Children() {
		if !strings.HasSuffix(name, "/") {
			continue
		}

		var upper *tree.MemTree

		if r.upper != nil {
			upper, _ = r.upper.Child(name)
		}

		cr := ruleOverlay{lower.(*tree.MemTree), upper}

		ds.Children[name] = cr.getSummary()
	}

	return ds
}

func (r *ruleOverlay) getSummary() *DirSummary {
	layer := cmp.Or(r.upper, r.lower)
	sr := byteio.StickyLittleEndianReader{Reader: bytes.NewReader(layer.Data())}
	ds := &DirSummary{
		Children: make(map[string]*DirSummary),
	}

	ds.uid = uint32(sr.ReadUintX())
	ds.gid = uint32(sr.ReadUintX())

	ds.RuleSummaries = make([]Rule, sr.ReadUintX())

	for n := range ds.RuleSummaries {
		ds.RuleSummaries[n].ID = sr.ReadUintX()
		ds.RuleSummaries[n].Users = readStats(&sr, users.Username)
		ds.RuleSummaries[n].Groups = readStats(&sr, users.Group)
	}

	return ds
}

// Stats represents the summarised stats for a particular user or group for a
// directory.
type Stats struct {
	id    uint32
	Name  string
	MTime uint64
	Files uint64
	Size  uint64
}

// ID returns the UID or GID for the summarised stats.
func (s *Stats) ID() uint32 {
	return s.id
}

func (s *Stats) writeTo(sw *byteio.StickyLittleEndianWriter) {
	sw.WriteUintX(uint64(s.id))
	sw.WriteUintX(uint64(s.MTime))
	sw.WriteUintX(uint64(s.Files))
	sw.WriteUintX(s.Size)
}

func readStats(br *byteio.StickyLittleEndianReader, name func(uint32) string) []Stats {
	stats := make([]Stats, br.ReadUintX())

	for n := range stats {
		stats[n] = Stats{
			id:    uint32(br.ReadUintX()),
			MTime: br.ReadUintX(),
			Files: br.ReadUintX(),
			Size:  br.ReadUintX(),
		}

		stats[n].Name = name(stats[n].id)
	}

	return stats
}
