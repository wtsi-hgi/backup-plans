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

type DirSummary struct {
	uid, gid      uint32
	ClaimedBy     string
	RuleSummaries []Rule
	Children      map[string]*DirSummary
}

func (d *DirSummary) MergeRules(rules []Rule) {
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

func (d *DirSummary) IDs() (uint32, uint32) {
	return d.uid, d.gid
}

func setNames(rules ruleStats, name func(uint32) string) {
	for n := range rules {
		rules[n].Name = name(rules[n].id)
	}
}

type RuleOverlay struct {
	lower, upper *tree.MemTree
}

func (r *RuleOverlay) Summary(path string) (*DirSummary, error) {
	if path == "" {
		return r.getSummaryWithChildren(), nil
	}

	cr, rest, err := r.getChild(path)
	if err != nil {
		return nil, err
	}

	return cr.Summary(rest)
}

func (r *RuleOverlay) getChild(path string) (*RuleOverlay, string, error) {
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

	cr := &RuleOverlay{lower, upper}

	return cr, path[pos+1:], nil
}

func (r *RuleOverlay) GetOwner(path string) (uint32, uint32, error) {
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

func (r *RuleOverlay) getOwner() (uint32, uint32) {
	sr := byteio.StickyLittleEndianReader{Reader: bytes.NewReader(cmp.Or(r.upper, r.lower).Data())}

	return uint32(sr.ReadUintX()), uint32(sr.ReadUintX())
}

func (r *RuleOverlay) getSummaryWithChildren() *DirSummary {
	ds := r.getSummary()

	for name, lower := range r.lower.Children() {
		if !strings.HasSuffix(name, "/") {
			continue
		}

		var upper *tree.MemTree

		if r.upper != nil {
			upper, _ = r.upper.Child(name)
		}

		cr := RuleOverlay{lower.(*tree.MemTree), upper}

		ds.Children[name] = cr.getSummary()
	}

	return ds
}

func (r *RuleOverlay) getSummary() *DirSummary {
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

type Stats struct {
	id    uint32
	Name  string
	MTime uint64
	Files uint64
	Size  uint64
}

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
