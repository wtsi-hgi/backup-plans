/*******************************************************************************
 * Copyright (c) 2025 Genome Research Ltd.
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

package config

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	ib "github.com/wtsi-hgi/backup-plans/internal/ibackup"
	"github.com/wtsi-hgi/backup-plans/internal/testirods"
)

func TestConfig(t *testing.T) {
	Convey("Given a valid YAML file and config files", t, func() {
		So(testirods.AddPseudoIRODsToolsToPathIfRequired(t), ShouldBeNil)

		servers := make(map[string]ibackup.ServerDetails)

		for i := range 2 {
			_, addr, certPath, dfn, err := ib.NewTestIbackupServer(t)
			So(err, ShouldBeNil)

			Reset(func() { dfn() }) //nolint:errcheck

			servers["example_"+strconv.Itoa(i+1)] = ibackup.ServerDetails{
				Addr:  addr,
				Cert:  certPath,
				Token: filepath.Join(filepath.Dir(certPath), ".ibackup.token"),
			}
		}

		tmp := t.TempDir()
		y := yamlConfig{
			IBackup: ibackup.Config{
				Servers: servers,
				PathToServer: map[string]ibackup.ServerTransformer{
					"^/some/path/": {
						ServerName:  "example_1",
						Transformer: ib.CustomTransformer,
					},
					"^/some/other/path/": {
						ServerName:  "example_2",
						Transformer: "prefix=/some/other/path/:/remote/other/path/",
					},
				},
			},
			OwnersFile:     filepath.Join(tmp, "owners"),
			BOMFile:        filepath.Join(tmp, "bom"),
			ReportingRoots: []string{"abc", "def"},
			AdminGroup:     123,
		}
		cfgFile := filepath.Join(tmp, "config.yml")

		f, err := os.Create(cfgFile)
		So(err, ShouldBeNil)
		So(yaml.NewEncoder(f).Encode(y), ShouldBeNil)

		u, err := user.Current()
		So(err, ShouldBeNil)

		group, err := user.LookupGroupId(u.Gid)
		So(err, ShouldBeNil)

		second, err := user.LookupGroupId("1")
		So(err, ShouldBeNil)

		So(os.WriteFile(y.OwnersFile, []byte(u.Gid+",ownerA\n0,ownerB\n"+second.Gid+",ownerB"), 0600), ShouldBeNil)
		So(os.WriteFile(y.BOMFile, []byte("group1,bomA\ngroup2,bomB\ngroup3,bomB"), 0600), ShouldBeNil)

		Convey("You can parse the file into a Config type", func() {
			config, err := ParseConfig(cfgFile)
			So(err, ShouldBeNil)

			So(config.GetOwners(), ShouldResemble, map[string][]string{
				"ownerA": {group.Name},
				"ownerB": {"root", second.Name},
			})
			So(config.GetBOMs(), ShouldResemble, map[string][]string{
				"bomA": {"group1"},
				"bomB": {"group2", "group3"},
			})
			So(config.GetReportingRoots(), ShouldResemble, y.ReportingRoots)
			So(config.GetAdminGroup(), ShouldEqual, y.AdminGroup)

			Convey("", func() {
				u, err := user.Current()
				So(err, ShouldBeNil)

				setName := "mySet"

				ib := config.GetIBackupClient()

				Reset(func() { ib.Stop() })

				So(ib.Backup("/some/path/a/dir/", setName, u.Username,
					[]string{"/some/path/a/dir/file", "/some/path/a/dir/file2"}, 0, 1, 2), ShouldBeNil)
				So(ib.Backup("/some/other/path/a/dir/", setName, u.Username,
					[]string{"/some/other/path/a/dir/file"}, 0, 3, 4), ShouldBeNil)

				baa, err := ib.GetBackupActivity("/some/path/a/dir/", setName, u.Username)
				So(err, ShouldBeNil)

				bab, err := ib.GetBackupActivity("/some/other/path/a/dir/", setName, u.Username)
				So(err, ShouldBeNil)

				So(baa.LastSuccess, ShouldNotEqual, bab.LastSuccess)

				mc := config.GetCachedIBackupClient()

				Reset(func() { mc.Stop() })

				ba, err := mc.GetBackupActivity("/some/path/a/dir/", setName, u.Username)
				So(err, ShouldBeNil)
				So(ba, ShouldResemble, baa)
			})
		})
	})
}

func TestOwners(t *testing.T) {
	Convey("An owners file can be correctly parsed in to a map", t, func() {
		u, err := user.Current()
		So(err, ShouldBeNil)

		group, err := user.LookupGroupId(u.Gid)
		So(err, ShouldBeNil)

		second, err := user.LookupGroupId("1")
		So(err, ShouldBeNil)

		m, err := parseOwners(strings.NewReader(``))
		So(err, ShouldBeNil)
		So(m, ShouldBeEmpty)

		m, err = parseOwners(strings.NewReader(u.Gid + ",ownerA"))
		So(err, ShouldBeNil)
		So(m, ShouldResemble, map[string][]string{
			"ownerA": {group.Name},
		})

		m, err = parseOwners(strings.NewReader(u.Gid + ",ownerA\n" + second.Gid + ",ownerA"))
		So(err, ShouldBeNil)
		So(m, ShouldResemble, map[string][]string{
			"ownerA": {group.Name, second.Name},
		})

		m, err = parseOwners(strings.NewReader(u.Gid + ",ownerA\n0,ownerB\n" + second.Gid + ",ownerB"))
		So(err, ShouldBeNil)
		So(m, ShouldResemble, map[string][]string{
			"ownerA": {group.Name},
			"ownerB": {"root", second.Name},
		})
	})
}

func TestBOM(t *testing.T) {
	Convey("A bom file can be correctly parsed in to a map", t, func() {
		m, err := parseBOM(strings.NewReader(``))
		So(err, ShouldBeNil)
		So(m, ShouldBeEmpty)

		m, err = parseBOM(strings.NewReader(`group1,bomA`))
		So(err, ShouldBeNil)
		So(m, ShouldResemble, map[string][]string{
			"bomA": {"group1"},
		})

		m, err = parseBOM(strings.NewReader(`` +
			`group1,bomA
group2,bomA`,
		))
		So(err, ShouldBeNil)
		So(m, ShouldResemble, map[string][]string{
			"bomA": {"group1", "group2"},
		})
		m, err = parseBOM(strings.NewReader(`` +
			`group1,bomA
group2,bomB
group3,bomB`,
		))
		So(err, ShouldBeNil)
		So(m, ShouldResemble, map[string][]string{
			"bomA": {"group1"},
			"bomB": {"group2", "group3"},
		})
	})
}
