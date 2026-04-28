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

package treegen

import (
	"bytes"
	"context"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/kuleuven/iron/api"
	"github.com/kuleuven/iron/msg"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/internal/iter"
	"github.com/wtsi-hgi/ibackup/transformer"
	"vimagination.zapto.org/byteio"
	"vimagination.zapto.org/tree"
)

func TestBackups(t *testing.T) {
	Convey("With a mock API connection", t, func() {
		txA, err := transformer.MakePathTransformer(`prefix=/local/files/:/remote/backups/collectionA/`)
		So(err, ShouldBeNil)

		txB, err := transformer.MakePathTransformer(`prefix=/local/files/:/remote/backups/collectionB/`)
		So(err, ShouldBeNil)

		m := mockConn{
			"/remote/backups/collectionA/": {
				{
					set: "/local/files/A/",
					tx:  txA,
					files: []file{
						{
							Path: "/local/files/A/myFile.txt",
							Size: 100,
						},
						{
							Path: "/local/files/A/myOtherFile.txt",
							Size: 200,
						},
					},
				},
				{
					set: "/local/files/B/",
					tx:  txA,
					files: []file{
						{
							Path: "/local/files/B/aFile.txt",
							Size: 300,
						},
						{
							Path: "/local/files/B/dir/bFile.txt",
							Size: 400,
						},
					},
				},
			},
			"/remote/backups/collectionB/": {
				{
					set: "/local/files/C/",
					tx:  txB,
					files: []file{
						{
							Path: "/local/files/C/hello.world",
							Size: 500,
						},
					},
				},
			},
			"/remote/backups/collectionUnused/": {
				{
					set: "/local/files/D/",
					tx:  txA,
					files: []file{
						{
							Path: "/local/files/D/bad.file",
							Size: 999999,
						},
					},
				},
			},
		}
		a := &api.API{Connect: func(_ context.Context) (api.Conn, error) { return m, nil }}

		Convey("You can build a backedup file tree", func() {
			n, err := processCollections(a, map[string]transformer.PathTransformer{
				"/remote/backups/collectionA/": txA,
				"/remote/backups/collectionB/": txB,
			})
			So(err, ShouldBeNil)

			var buf bytes.Buffer

			So(tree.Serialise(&buf, n), ShouldBeNil)

			bt, err := tree.OpenMem(buf.Bytes())
			So(err, ShouldBeNil)

			for path, countSize := range map[string]sizeCount{
				"":                               {1500, 5},
				"local/":                         {1500, 5},
				"local/files/":                   {1500, 5},
				"local/files/A/":                 {300, 2},
				"local/files/B/":                 {700, 2},
				"local/files/C/":                 {500, 1},
				"local/files/A//":                {300, 2},
				"local/files/B//":                {700, 2},
				"local/files/B//dir/":            {400, 1},
				"local/files/A//myFile.txt":      {100, 0},
				"local/files/A//myOtherFile.txt": {200, 0},
				"local/files/B//aFile.txt":       {300, 0},
				"local/files/B//dir/bFile.txt":   {400, 0},
				"local/files/C//hello.world":     {500, 0},
			} {
				l := traverseTree(t, bt, path)
				SoMsg(path, l, ShouldNotBeNil)

				d := byteio.MemLittleEndian(l.Data())

				SoMsg(path+": size", d.ReadUintX(), ShouldEqual, countSize.size)
				SoMsg(path+": count", d.ReadUintX(), ShouldEqual, countSize.count)
			}

			So(traverseTree(t, bt, "local/files/D/"), ShouldBeNil)
		})
	})
}

func traverseTree(t *testing.T, m *tree.MemTree, path string) *tree.MemTree {
	t.Helper()

	var err error

	for part := range iter.FilePathParts(path) {
		m, err = m.Child(part)
		if err != nil {
			return nil
		}
	}

	return m
}

type file struct {
	Path string
	Size uint64
}

type set struct {
	set   string
	tx    transformer.PathTransformer
	files []file
}

type mockConn map[string][]set

func (mockConn) ClientSignature() string                                      { return "testsignature" }
func (mockConn) NativePassword() string                                       { return "testpassword" }
func (mockConn) Close() error                                                 { return nil }
func (mockConn) RegisterCloseHandler(handler func() error) context.CancelFunc { return func() {} }
func (m mockConn) RequestWithBuffers(ctx context.Context, apiNumber msg.APINumber,
	request, response any, requestBuf, responseBuf []byte) error {
	return nil
}

func (m mockConn) Request(ctx context.Context, apiNumber msg.APINumber, request, response any) error {
	req := request.(*msg.QueryRequest)    //nolint:errcheck,forcetypeassert
	resp := response.(*msg.QueryResponse) //nolint:errcheck,forcetypeassert

	collection := getCollection(req)
	if collection == "" {
		return ErrNoCollection
	}

	coll, ok := m[collection]
	if !ok {
		return ErrNoCollection
	}

	resp.AttributeCount = 4
	resp.SQLResult = []msg.SQLResult{
		{AttributeIndex: msg.ICAT_COLUMN_COLL_NAME},
		{AttributeIndex: msg.ICAT_COLUMN_DATA_NAME},
		{AttributeIndex: msg.ICAT_COLUMN_META_DATA_ATTR_VALUE},
		{AttributeIndex: msg.ICAT_COLUMN_DATA_SIZE},
	}

	for _, set := range coll {
		for _, file := range set.files {
			resp.RowCount++

			filePath, err := set.tx(file.Path)
			So(err, ShouldBeNil)

			resp.SQLResult[0].Values = append(resp.SQLResult[0].Values, path.Dir(filePath))
			resp.SQLResult[1].Values = append(resp.SQLResult[1].Values, path.Base(filePath))
			resp.SQLResult[2].Values = append(resp.SQLResult[2].Values, "plan::"+set.set)
			resp.SQLResult[3].Values = append(resp.SQLResult[3].Values, strconv.FormatUint(file.Size, 10))
		}
	}

	return nil
}

func getCollection(req *msg.QueryRequest) string {
	for n, k := range req.Conditions.Keys {
		if k == msg.ICAT_COLUMN_COLL_NAME.Int() {
			_, coll, _ := strings.Cut(req.Conditions.Values[n], " ")

			return strings.TrimPrefix(strings.TrimSuffix(strings.TrimSuffix(coll, "'"), "%"), "'")
		}
	}

	return ""
}

var (
	ErrNoCollection = &msg.IRODSError{
		Code:    msg.CAT_NO_ROWS_FOUND,
		Message: "CAT_NO_ROWS_FOUND",
	}
)
