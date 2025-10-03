package ibackup

import (
	"errors"
	"regexp"
	"time"

	//TODO: replace use of backups and rules with a new pkg that figures out our
	//fofns using tree dbs and the sql plan db
	// "github.com/wtsi-hgi/backup-plans/backups"
	// "github.com/wtsi-hgi/backup-plans/db/rules"

	gas "github.com/wtsi-hgi/go-authserver"
	"github.com/wtsi-hgi/ibackup/server"
	"github.com/wtsi-hgi/ibackup/set"
)

const SetNamePrefix = "plan::"

var (
	isHumgen = regexp.MustCompile(`^/lustre/scratch[0-9]+/humgen/`)
	isGengen = regexp.MustCompile(`^/lustre/scratch[0-9]+/gengen/`)
	isOtar   = regexp.MustCompile(`^/lustre/scratch[0-9]+/open-targets/`)

	ErrInvalidPath = errors.New("cannot determine transformer from path")

	toBackup = make(map[string]struct{})
)

// Connect returns a client that can talk to the given ibackup server using
// the .ibackup.jwt and .ibackup.token files.
func Connect(url, cert string) (*server.Client, error) {
	client, err := gas.NewClientCLI(".ibackup.jwt", ".ibackup.token", url, cert, false)
	if err != nil {
		return nil, err
	}

	jwt, err := client.GetJWT()
	if err != nil {
		return nil, err
	}

	return server.NewClient(url, cert, jwt), nil
}

// Backup creates a new set called setName for the requester if frequency > 0
// and it has been longer than the frequency since the last discovery for that
// set.
func Backup(client *server.Client, setName, requester string, files []string, frequency int) error {
	if len(files) == 0 || frequency == 0 {
		return nil
	}

	transformer := getTransformer(files[0])
	if transformer == "" {
		return ErrInvalidPath
	}

	got, err := client.GetSetByName(requester, SetNamePrefix+setName)
	if errors.Is(err, server.ErrBadSet) {
		got = &set.Set{
			Name:        SetNamePrefix + setName,
			Requester:   requester,
			Transformer: transformer,
			Description: "automatic backup set",
			Metadata:    map[string]string{},
			Failed:      0,
		}

		if err := client.AddOrUpdateSet(got); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if got.LastDiscovery.Add(time.Hour*24*time.Duration(frequency-1) + time.Hour*12).After(time.Now()) {
		return nil
	}

	if err := client.MergeFiles(got.ID(), files); err != nil {
		return err
	}

	toBackup[got.ID()] = struct{}{}

	return nil
}

func RunBackups(client *server.Client) error {
	for id := range toBackup {
		if err := client.TriggerDiscovery(id, false); err != nil {
			return err
		}
	}

	clear(toBackup)

	return nil
}

func getTransformer(file string) string {
	if isHumgen.MatchString(file) {
		return "humgen"
	}

	if isGengen.MatchString(file) {
		return "gengen"
	}

	if isOtar.MatchString(file) {
		return "otar"
	}

	return ""
}

// SetBackupActivity holds info about backup activity retrieved from an ibackup
// server.
type SetBackupActivity struct {
	LastSuccess time.Time
	Name        string
	Requester   string
	Failures    uint64
}

// GetBackupActivity queires an ibackup server to get the last completed backup
// date and number of failures for the given set name and requester.
func GetBackupActivity(client *server.Client, setName, requester string) (*SetBackupActivity, error) {
	var (
		sba SetBackupActivity
		err error
	)

	sba.LastSuccess, err = GetSetLastCompleted(client, setName, requester)
	if err != nil {
		return nil, err
	}

	sba.Name = setName
	sba.Requester = requester

	got, err := client.GetSetByName(requester, setName)
	if err != nil && err != server.ErrBadSet {
		return nil, err
	}

	if got != nil {
		sba.Failures = got.Failed
	}

	return &sba, nil
}

// GetBackupActivity queries an ibackup server to get the last completed backup
// date for each set that corresponds to the given fofns (eg. as retrieved by
// backups.New().Fofns()).
// fofns iter.Seq2[backups.ProjectAction, []string]
// TODO: have a new thing that gives us fofns, which are rules for a name+requester?
// func GetBackupActivity(client *server.Client, rules []db.Rule) (BackupActivity, error) {
// 	var ba BackupActivity

// 	for _, rule := range rules {
// 		var setName string
// 		var err error
// 		var bs SetBackupActivity

// 		//TODO: convert these Action types to db types like BackupNone etc.
// 		if rule.Metadata == "" && rule.BackupType == db.BackupManual {
// 			continue
// 		}

// 		if rule.BackupType == db.BackupManual {
// 			setName = rule.Metadata
// 		} else {
// 			setName = SetNamePrefix + "tempName" //TODO: get a name from somewhere?? action.Name
// 		}

// 		bs.LastSuccess, err = GetSetLastCompleted(client, setName, "tempRequester") //TODO: get a requestor from somewhere?? action.Requestor
// 		if err != nil && err != server.ErrBadSet {
// 			return nil, err
// 		}

// 		bs.Name = "tempName"           //TODO: get a name from somewhere?? action.Name
// 		bs.Requester = "tempRequester" //TODO: get a requestor from somewhere?? action.Requestor

// 		got, err := client.GetSetByName(bs.Requester, setName)
// 		if err != nil && err != server.ErrBadSet {
// 			return nil, err
// 		}

// 		if got != nil {
// 			bs.Failures = got.Failed
// 		}

// 		ba = append(ba, bs)
// 	}
// 	return ba, nil
// }

// GetSetLastCompleted finds the given set by name and returns its LastCompelted
// time.
func GetSetLastCompleted(client *server.Client, setName, requester string) (time.Time, error) {
	got, err := client.GetSetByName(requester, setName)
	if err != nil {
		return time.Time{}, err
	}

	return got.LastCompleted, nil
}
