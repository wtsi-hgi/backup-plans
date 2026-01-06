package config

import (
	"encoding/csv"
	"errors"
	"io"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	"github.com/wtsi-hgi/backup-plans/users"
)

const csvCols = 2

type yamlConfig struct {
	IBackup              ibackup.Config
	IBackupCacheDuration uint64
	BOMFile              string
	OwnersFile           string
	ReportingRoots       []string
	AdminGroup           uint32
	ReloadTime           uint64
}

type Config struct {
	path string

	mu                  sync.RWMutex
	ibackupClient       *ibackup.MultiClient
	ibackupCachedClient *ibackup.MultiCache
	boms                map[string][]string
	owners              map[string][]string
	yamlConfig          yamlConfig
}

func ParseConfig(path string) (*Config, error) {
	c := &Config{path: path}

	if err := c.loadConfig(); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Config) loadConfig() error {
	defer c.scheduleReload()

	f, err := os.Open(c.path)
	if err != nil {
		return err
	}

	defer f.Close()

	err = yaml.NewDecoder(f).Decode(&c.yamlConfig)
	if err != nil {
		return err
	}

	if err = c.loadIBackup(); err != nil {
		return err
	}

	if err = c.loadBOMs(); err != nil {
		return err
	}

	if err = c.loadOwners(); err != nil {
		return err
	}

	return nil
}

func (c *Config) scheduleReload() {
	if c.yamlConfig.ReloadTime == 0 {
		return
	}

	go c.reload()
}

func (c *Config) reload() {
	time.Sleep(time.Second * time.Duration(c.yamlConfig.ReloadTime))

	c.loadConfig()
}

func (c *Config) loadIBackup() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	mc, err := ibackup.New(c.yamlConfig.IBackup)
	if err != nil {
		return err
	}

	if c.ibackupCachedClient == nil {
		c.ibackupCachedClient = ibackup.NewMultiCache(mc, time.Second*time.Duration(c.yamlConfig.IBackupCacheDuration))
	} else {
		c.ibackupCachedClient.Update(mc)
	}

	if c.ibackupClient != nil {
		c.ibackupClient.Stop()
	}

	c.ibackupClient = mc

	return nil
}

func (c *Config) loadBOMs() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.yamlConfig.BOMFile == "" {
		return nil
	}

	f, err := os.Open(c.yamlConfig.BOMFile)
	if err != nil {
		return err
	}

	defer f.Close()

	bomMap, err := parseBOM(f)
	if err != nil {
		return err
	}

	c.boms = bomMap

	return nil
}

func parseBOM(r io.Reader) (map[string][]string, error) {
	bomMap := make(map[string][]string)

	cr := csv.NewReader(r)

	for {
		record, err := cr.Read()
		if errors.Is(err, io.EOF) { //nolint:gocritic,nestif
			break
		} else if err != nil {
			return nil, err
		} else if len(record) < csvCols {
			continue
		}

		bomMap[record[1]] = append(bomMap[record[1]], record[0])
	}

	return bomMap, nil
}

func (c *Config) loadOwners() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.yamlConfig.OwnersFile == "" {
		return nil
	}

	f, err := os.Open(c.yamlConfig.OwnersFile)
	if err != nil {
		return err
	}

	defer f.Close()

	ownersMap, err := parseOwners(f)
	if err != nil {
		return err
	}

	c.owners = ownersMap

	return nil
}

func parseOwners(r io.Reader) (map[string][]string, error) {
	ownersMap := make(map[string][]string)

	cr := csv.NewReader(r)

	for {
		record, err := cr.Read()
		if errors.Is(err, io.EOF) { //nolint:gocritic,nestif
			break
		} else if err != nil {
			return nil, err
		} else if len(record) < csvCols {
			continue
		}

		gid, err := strconv.ParseUint(record[0], 10, 32)
		if err != nil {
			return nil, err
		}

		ownersMap[record[1]] = append(ownersMap[record[1]], users.Group(uint32(gid)))
	}

	return ownersMap, nil
}

func (c *Config) GetIBackupClient() *ibackup.MultiClient {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.ibackupClient
}

func (c *Config) GetCachedIBackupClient() *ibackup.MultiCache {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.ibackupCachedClient
}

func (c *Config) GetBOMs() map[string][]string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.boms
}

func (c *Config) GetOwners() map[string][]string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.owners
}

func (c *Config) GetReportingRoots() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.yamlConfig.ReportingRoots
}

func (c *Config) GetAdminGroup() uint32 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.yamlConfig.AdminGroup
}
