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

var (
	isHumgen = regexp.MustCompile(`^/lustre/scratch[0-9]+/humgen/`)
	isGengen = regexp.MustCompile(`^/lustre/scratch[0-9]+/gengen/`)
	isOtar   = regexp.MustCompile(`^/lustre/scratch[0-9]+/open-targets/`)

	ErrInvalidPath = errors.New("cannot determine transformer from path")
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
func Backup(client *server.Client, setName, requester string, files []string, frequency int) (string, error) {
	if len(files) == 0 || frequency == 0 {
		return "", nil
	}

	transformer := getTransformer(files[0])
	if transformer == "" {
		return "", ErrInvalidPath
	}

	got, err := client.GetSetByName(requester, setName)
	if errors.Is(err, server.ErrBadSet) {
		got = &set.Set{
			Name:        setName,
			Requester:   requester,
			Transformer: transformer,
			Metadata:    map[string]string{},
			Failed:      0,
		}

		if err := client.AddOrUpdateSet(got); err != nil {
			return "", err
		}

	} else if err != nil {
		return "", err
	} else if got.LastDiscovery.Add(time.Hour*24*time.Duration(frequency-1) + time.Hour*12).After(time.Now()) {
		return "", nil
	}

	if err := client.MergeFiles(got.ID(), files); err != nil {
		return "", err
	}

	return got.ID(), nil
}

func RunBackups(setIDs []string, client *server.Client) error {
	for _, id := range setIDs {
		if id == "" {
			continue
		}

		if err := client.TriggerDiscovery(id, false); err != nil {
			return err
		}
	}

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

// GetBackupActivity queries an ibackup server to get the last completed backup
// date and number of failures for the given set name and requester.
func GetBackupActivity(client *server.Client, setName, requester string) (*SetBackupActivity, error) {
	var (
		sba SetBackupActivity
		err error
	)

	sba.Name = setName
	sba.Requester = requester

	got, err := client.GetSetByName(requester, setName)
	if err != nil {
		return nil, err
	}

	if got != nil {
		sba.Failures = got.Failed
		sba.LastSuccess = got.LastCompleted
	}

	return &sba, nil
}
