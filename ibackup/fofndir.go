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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"iter"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wtsi-hgi/ibackup/fofn"
)

const (
	fullwidthSolidus           = "／"
	safeNameMaxComponentBytes  = 240
	safeNameHashHexChars       = 16
	safeNameHashSeparator      = "--"
	fofnDirPerm                = 0o755
	frequencyWindowOffsetHours = 12
)

// FofnDirWriter writes null-terminated FOFN files and config.yml files into
// subdirectories under a base fofn directory.
type FofnDirWriter struct {
	baseDir string
}

// NewFofnDirWriter returns a new FofnDirWriter that creates subdirectories
// under baseDir.
func NewFofnDirWriter(baseDir string) *FofnDirWriter {
	return &FofnDirWriter{baseDir: baseDir}
}

// Write creates (or updates) the fofn subdirectory for the given set and
// writes the null-terminated FOFN and config.yml.
func (f *FofnDirWriter) Write(setName string, transformer string, files iter.Seq[string],
	frequency uint, metadata map[string]string) (bool, error) {
	shouldWrite, err := f.shouldWrite(setName, frequency)
	if err != nil {
		return false, err
	}

	if !shouldWrite {
		return false, nil
	}

	subDir, wrote, err := f.writeFofn(setName, files)
	if err != nil {
		return false, err
	}

	if !wrote {
		return false, nil
	}

	err = writeConfig(subDir, transformer, frequency == 0, metadata)
	if err != nil {
		return false, err
	}

	return true, nil
}

// UpdateConfig updates the config.yml for an existing set directory without
// rewriting its FOFN file.
func (f *FofnDirWriter) UpdateConfig(setName string, transformer string,
	freeze bool, metadata map[string]string) error {
	subDir := filepath.Join(f.baseDir, SafeName(setName))

	err := ensureConfigTargetExists(subDir)
	if err != nil {
		return err
	}

	return writeConfig(subDir, transformer, freeze, metadata)
}

// SafeName converts a set name to a directory-safe name by removing a
// "plan::" prefix and replacing forward slashes.
func SafeName(setName string) string {
	setName = strings.TrimPrefix(setName, "plan::")
	safeName := strings.ReplaceAll(setName, "/", fullwidthSolidus)

	if len([]byte(safeName)) <= safeNameMaxComponentBytes {
		return safeName
	}

	hashSuffix := safeNameHashSeparator + shortSHA256Hex(safeName)
	prefixLimit := safeNameMaxComponentBytes - len(hashSuffix)

	return truncateToUTF8ByteLength(safeName, prefixLimit) + hashSuffix
}

func ensureConfigTargetExists(subDir string) error {
	_, err := os.Stat(filepath.Join(subDir, "config.yml"))

	return err
}

func writeConfig(subDir, transformer string, freeze bool,
	metadata map[string]string) error {
	return fofn.WriteConfig(subDir, fofn.SubDirConfig{
		Transformer: transformer,
		Freeze:      freeze,
		Metadata:    metadata,
	})
}

func (f *FofnDirWriter) shouldWrite(setName string, frequency uint) (bool, error) {
	fofnPath := filepath.Join(f.baseDir, SafeName(setName), "fofn")

	stat, err := os.Stat(fofnPath)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}

		return false, err
	}

	if frequency == 0 {
		return false, nil
	}

	window := time.Hour*24*time.Duration(frequency) - //nolint:gosec // G115: frequency is small
		time.Hour*frequencyWindowOffsetHours

	if !stat.ModTime().Add(window).Before(time.Now()) {
		return false, nil
	}

	return true, nil
}

func (f *FofnDirWriter) writeFofn(setName string,
	files iter.Seq[string]) (subDir string, wrote bool, err error) {
	subDir = filepath.Join(f.baseDir, SafeName(setName))

	fofnFD, wrote, err := streamToFofn(subDir, files)
	if err != nil {
		return "", false, err
	}

	if !wrote {
		return subDir, false, nil
	}

	err = closeFofnFile(fofnFD)

	return subDir, true, err
}

func streamToFofn(subDir string, files iter.Seq[string]) (*os.File, bool, error) {
	var fofnFD *os.File

	wrote := false

	for path := range files {
		fd, err := ensureOpenFofnFile(fofnFD, subDir)
		if err != nil {
			return nil, false, err
		}

		fofnFD = fd

		if err := writePathToFofn(fofnFD, path); err != nil {
			return nil, false, err
		}

		wrote = true
	}

	return fofnFD, wrote, nil
}

func closeFofnFile(fofnFD *os.File) error {
	if fofnFD == nil {
		return nil
	}

	return fofnFD.Close()
}

func ensureOpenFofnFile(fofnFD *os.File, subDir string) (*os.File, error) {
	if fofnFD != nil {
		return fofnFD, nil
	}

	return createFofnFile(subDir)
}

func createFofnFile(subDir string) (*os.File, error) {
	err := os.MkdirAll(subDir, fofnDirPerm)
	if err != nil {
		return nil, err
	}

	return os.Create(filepath.Join(subDir, "fofn"))
}

func writePathToFofn(fofnFD *os.File, path string) error {
	err := writePath(fofnFD, path)
	if err == nil {
		return nil
	}

	if closeErr := closeFofnFile(fofnFD); closeErr != nil {
		return errors.Join(err, closeErr)
	}

	return err
}

func writePath(fofnFD *os.File, path string) error {
	_, err := fofnFD.WriteString(path)
	if err != nil {
		return err
	}

	_, err = fofnFD.WriteString("\x00")

	return err
}

func shortSHA256Hex(input string) string {
	hash := sha256.Sum256([]byte(input))
	encoded := hex.EncodeToString(hash[:])

	return encoded[:safeNameHashHexChars]
}

func truncateToUTF8ByteLength(input string, maxBytes int) string {
	if len(input) <= maxBytes {
		return input
	}

	for maxBytes > 0 && !utf8.RuneStart(input[maxBytes]) {
		maxBytes--
	}

	return input[:maxBytes]
}
