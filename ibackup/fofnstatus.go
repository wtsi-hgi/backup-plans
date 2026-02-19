/*******************************************************************************
 * Copyright (c) 2026 Genome Research Ltd.
 *
 * Author: Sendu Bala <sb10@sanger.ac.uk>
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
	"os"
	"path/filepath"

	"github.com/wtsi-hgi/ibackup/fofn"
)

// FofnStatusReader reads status files from fofn subdirectories.
type FofnStatusReader struct {
	baseDir string
}

// NewFofnStatusReader returns a reader that looks for status files under
// baseDir.
func NewFofnStatusReader(baseDir string) *FofnStatusReader {
	return &FofnStatusReader{baseDir: baseDir}
}

// GetBackupActivity reads the status file for the given set name and returns
// the mapped backup activity.
func (f *FofnStatusReader) GetBackupActivity(setName string) (*SetBackupActivity, error) {
	statusPath := filepath.Join(f.baseDir, SafeName(setName), "status")

	statusInfo, err := getStatusFileInfo(statusPath)
	if err != nil {
		return nil, err
	}

	if statusInfo == nil {
		return nil, nil //nolint:nilnil
	}

	_, counts, err := fofn.ParseStatus(statusPath)
	if err != nil {
		return nil, err
	}

	activity := mapStatusCounts(setName, counts)

	if counts.Failed == 0 {
		activity.LastSuccess = statusInfo.ModTime()
	}

	return activity, nil
}

func getStatusFileInfo(statusPath string) (os.FileInfo, error) {
	statusInfo, err := os.Stat(statusPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil //nolint:nilnil
		}

		return nil, err
	}

	return statusInfo, nil
}

func mapStatusCounts(setName string, counts fofn.StatusCounts) *SetBackupActivity {
	return &SetBackupActivity{
		Name:       setName,
		Failures:   nonNegativeUint64(counts.Failed),
		Uploaded:   counts.Uploaded,
		Replaced:   counts.Replaced,
		Unmodified: counts.Unmodified,
		Missing:    counts.Missing,
		Frozen:     counts.Frozen,
		Orphaned:   counts.Orphaned,
		Warning:    counts.Warning,
		Hardlink:   counts.Hardlink,
	}
}

func nonNegativeUint64(count int) uint64 {
	if count < 0 {
		return 0
	}

	return uint64(count)
}
