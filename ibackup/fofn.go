/*******************************************************************************
 * Copyright (c) 2026 Genome Research Ltd.
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

package ibackup

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/wtsi-hgi/ibackup/fofn"
	"github.com/wtsi-hgi/ibackup/server"
	"github.com/wtsi-hgi/ibackup/set"
	"github.com/wtsi-hgi/ibackup/transfer"
)

const (
	statusFile     = "status"
	fofnFile       = "fofn"
	fullwidthColon = "\uff1a"
)

type FOFNClient struct {
	base string
}

func NewFOFNClient(path string) *FOFNClient {
	return &FOFNClient{base: path}
}

func (fc *FOFNClient) path(id string, filename string) string {
	return filepath.Join(fc.base, id, filename)
}

func (fc *FOFNClient) GetSetByName(requester, setName string) (*set.Set, error) {
	s := &set.Set{Name: setName, Requester: requester}

	config, err := fofn.ReadConfig(fc.path(s.ID(), ""))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, server.ErrBadSet
	}

	statusFile := fc.path(s.ID(), statusFile)
	_, counts, _ := fofn.ParseStatus(statusFile)

	completed, err := os.Stat(statusFile)
	if err == nil {
		s.LastCompleted = completed.ModTime()

		if s.LastCompleted.After(s.LastDiscovery) {
			s.Status = set.Complete
		}
	}

	s.Uploaded = uint64(counts.Uploaded)
	s.Replaced = uint64(counts.Replaced)
	s.Missing = uint64(counts.Missing)
	s.Failed = uint64(counts.Failed)
	s.Orphaned = uint64(counts.Orphaned)
	s.Hardlinks = uint64(counts.Hardlink)
	s.Transformer = config.Transformer
	s.Frozen = config.Freeze
	s.Metadata = swapMetadataKeys(config.Metadata, fullwidthColon, ":")

	return s, nil
}

func swapMetadataKeys(m map[string]string, replace, with string) map[string]string {
	newMap := make(map[string]string, len(m))

	for k, v := range m {
		newMap[strings.ReplaceAll(k, replace, with)] = v
	}

	return newMap
}

func (fc *FOFNClient) AddOrUpdateSet(set *set.Set) error {
	fofnPath := fc.path(set.ID(), "")

	if err := os.MkdirAll(fofnPath, 0755); err != nil {
		return err
	}

	if set.Metadata == nil {
		set.Metadata = make(map[string]string)
	}

	set.Metadata[transfer.MetaKeySets] = set.Name
	set.Metadata[transfer.MetaKeyOwner] = set.Requester

	return fofn.WriteConfig(fofnPath, fofn.SubDirConfig{
		Transformer: set.Transformer,
		Metadata:    swapMetadataKeys(set.Metadata, ":", fullwidthColon),
		Freeze:      set.Frozen,
	})
}

func (fc *FOFNClient) MergeFiles(setID string, paths []string) (err error) {
	var f *os.File

	f, err = os.Create(fc.path(setID, fofnFile+".tmp"))
	if err != nil {
		return err
	}

	defer func() {
		if errr := f.Close(); err == nil {
			err = errr
		}
	}()

	b := bufio.NewWriter(f)

	for _, path := range paths {
		if _, err = b.WriteString(path); err != nil {
			return err
		}

		if err = b.WriteByte(0); err != nil {
			return err
		}
	}

	return b.Flush()
}

func (fc *FOFNClient) TriggerDiscovery(setID string, forceRemovals bool) error {
	fofnPath := fc.path(setID, fofnFile)

	return os.Rename(fofnPath+".tmp", fofnPath)
}
