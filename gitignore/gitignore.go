package gitignore

import (
	"bufio"
	"io"
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

		if strings.HasSuffix(rule.Match, "/") {
			rule.Match += "*"
		}

		rules = append(rules, rule)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return rules, nil
}

// FromRules returns a gitignore file contents string, given a gitignore
// object.
func FromRules(rules []db.Rule) (string, error) {
	var matches []string

	for _, rule := range rules {
		match := rule.Match

		if rule.BackupType != db.BackupNone {
			match = "!" + match
		}

		matches = append(matches, match)
	}

	output := strings.Join(matches, "\n")

	return output, nil
}
