package gitignore

import (
	"bufio"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/wtsi-hgi/backup-plans/db"
)

type Config struct {
	BackupType db.BackupType
	Frequency  uint
	Metadata   string
	ReviewDate time.Time
	RemoveDate time.Time
}

// ToRules accepts a gitignore file reader, and corresponding config data and
// returns rules. A default rule to backup * is always added.
func ToRules(r io.Reader, config Config) ([]db.Rule, error) {
	var rules []db.Rule

	rules = append(rules, db.Rule{
		BackupType: config.BackupType,
		Frequency:  config.Frequency,
		Metadata:   config.Metadata,
		ReviewDate: config.ReviewDate,
		RemoveDate: config.RemoveDate,
		Match:      "*",
	})

	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "#") || scanner.Text() == "" {
			continue
		}

		rule := db.Rule{
			Frequency:  config.Frequency,
			Metadata:   config.Metadata,
			ReviewDate: config.ReviewDate,
			RemoveDate: config.RemoveDate,
			Match:      scanner.Text(),
		}

		if strings.HasPrefix(rule.Match, "!") {
			rule.BackupType = config.BackupType
			rule.Match = strings.TrimPrefix(rule.Match, "!")
		}

		if rule.ForAnySubDir() {
			rule.Match = "/" + rule.Match
			if rule.ForDir() {
				rule.Match += "*"
			}
			rules = addRule(rules, rule)

			rule.Match = "*" + rule.Match
		} else if rule.ForDir() {
			rule.Match = rule.Match + "*"
		}

		rules = addRule(rules, rule)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return rules, nil
}

// FromRules returns a gitignore file contents string, given a gitignore
// object.
func FromRules(w io.Writer, rules []db.Rule) (io.Writer, error) {
	var matches []string
	for _, rule := range rules {
		match := rule.Match

		if rule.BackupType != db.BackupNone {
			match = "!" + match
		}

		matches = append(matches, match)
	}

	output := strings.Join(matches, "\n")
	_, err := w.Write([]byte(output))
	if err != nil {
		return nil, err
	}

	return w, nil
}

func addRule(rules []db.Rule, rule db.Rule) (output []db.Rule) {
	if slices.Contains(rules, rule) {
		return rules
	}
	return append(rules, rule)
}
