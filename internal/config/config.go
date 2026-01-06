package config

import (
	"sync"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey" //nolint:staticcheck,revive
	"github.com/wtsi-hgi/backup-plans/config"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	internalibackup "github.com/wtsi-hgi/backup-plans/internal/ibackup"
)

func NewConfig(t *testing.T, boms, owners map[string][]string, rr []string, ag uint32) *config.Config {
	t.Helper()

	mc := internalibackup.NewMultiClient(t)

	var c config.Config

	sc := (*struct {
		_      string
		_      sync.RWMutex
		mc     *ibackup.MultiClient
		cc     *ibackup.MultiCache
		boms   map[string][]string
		owners map[string][]string
		_      ibackup.Config
		_      uint64
		_, _   string
		rr     []string
		ag     uint32
	})(unsafe.Pointer(&c))

	sc.mc = mc
	sc.cc = ibackup.NewMultiCache(mc, 3600)
	sc.boms = boms
	sc.owners = owners
	sc.rr = rr
	sc.ag = ag

	Reset(func() {
		mc.Stop()
		sc.cc.Stop()
	})

	return &c
}
