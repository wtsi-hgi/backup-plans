/*******************************************************************************
 * Copyright (c) 2025 Genome Research Ltd.
 *
 * Author: Sky Haines <sh55@sanger.ac.uk>
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

package testirods

import (
	_ "embed"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

//go:embed pseudo
var pseudo []byte

var exes = [...]string{"baton-do", "ils", "iget", "iput", "irm"} //nolint:gochecknoglobals

func AddPseudoIRODsToolsToPathIfRequired(t *testing.T) error {
	t.Helper()

	if env := os.Getenv("IBACKUP_TEST_COLLECTION"); env == "" {
		return addToolsToPath(t)
	}

	for _, exe := range exes {
		if _, err := exec.LookPath(exe); err != nil {
			return addToolsToPath(t)
		}
	}

	return nil
}

func addToolsToPath(t *testing.T) error {
	t.Helper()

	if env := os.Getenv("IBACKUP_TEST_COLLECTION"); env == "" {
		os.Setenv("IBACKUP_TEST_COLLECTION", "/irods/test")
	}

	dir := t.TempDir()
	pseudoPath := filepath.Join(dir, exes[0])

	if err := os.WriteFile(pseudoPath, pseudo, 0700); err != nil { //nolint:gosec,mnd
		return err
	}

	for _, exe := range exes[1:] {
		if err := os.Link(pseudoPath, filepath.Join(dir, exe)); err != nil {
			return err
		}
	}

	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("IRODS_BASE", t.TempDir())

	return nil
}
