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
// returns rules.
func ToRules(r io.Reader, config Config) ([]db.Rule, error) {
	var rules []db.Rule

	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		rule := db.Rule{
			Frequency:  config.Frequency,
			Metadata:   config.Metadata,
			ReviewDate: config.ReviewDate,
			RemoveDate: config.RemoveDate,
			Match:      scanner.Text(),
		}

		if strings.HasPrefix(rule.Match, "!") {
			rule.BackupType = config.BackupType
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
		matches = append(matches, rule.Match)
	}

	output := strings.Join(matches, "\n")

	return output, nil
}
