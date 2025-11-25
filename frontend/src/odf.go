//go:generate go run odf.go

package main

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"fmt"
	"hash/crc32"
	"io"
	"os"

	"vimagination.zapto.org/byteio"
)

const mime = "application/vnd.oasis.opendocument.spreadsheet"
const mimeName = "mimetype"
const manifest = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0" manifest:version="1.3"><manifest:file-entry manifest:full-path="/" manifest:media-type="application/vnd.oasis.opendocument.spreadsheet"/><manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/></manifest:manifest>`
const manifestName = "META-INF/manifest.xml"
const contentName = "content.xml"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)

		os.Exit(1)
	}
}

func run() error {
	var buf, cdBuf bytes.Buffer

	lw := byteio.StickyLittleEndianWriter{Writer: &buf}
	cdlw := byteio.StickyLittleEndianWriter{Writer: &cdBuf}

	writeFile(&lw, &cdlw, mimeName, mime, false)
	writeFile(&lw, &cdlw, manifestName, manifest, true)

	contentStart := lw.Count
	cdfhInsert := cdlw.Count

	writeFile(&lw, &cdlw, contentName, "", false)

	contentData := lw.Count
	cdfhInsert += lw.Count

	lw.Write(cdBuf.Bytes())

	writeEOCD(&lw, &cdlw)

	f, err := os.Create("odf_data.ts")
	if err != nil {
		return err
	}

	defer f.Close()

	fmt.Fprintf(
		f,
		"export const ods = Uint8Array.fromBase64(%q),\n\tcontentMetaInsert = %d,\n\tcontentData = %d,\n\tcdfhMetaInsert = %d;\n",
		base64.StdEncoding.EncodeToString(buf.Bytes()),
		contentStart+14,
		contentData,
		cdfhInsert+16,
	)

	return lw.Err
}

func writeFile(lw, cdlw *byteio.StickyLittleEndianWriter, name, contents string, compress bool) {
	data := []byte(contents)

	crc := crc32.ChecksumIEEE(data)

	var method uint16

	if compress {
		method = 8

		var buf bytes.Buffer

		f, _ := flate.NewWriter(&buf, flate.BestCompression)

		io.WriteString(f, manifest)
		f.Close()

		data = buf.Bytes()
	}

	cdlw.WriteUint32(0x02014B50)            // Central directory file Header
	cdlw.WriteUint16(0x14)                  // Version
	cdlw.WriteUint16(0x14)                  // Minimum Version
	cdlw.WriteUint16(0)                     // General Purpose Flags
	cdlw.WriteUint16(method)                // Compression Method
	cdlw.WriteUint32(0)                     // Modified time/date
	cdlw.WriteUint32(crc)                   // CRC
	cdlw.WriteUint32(uint32(len(contents))) // Uncompressed Size
	cdlw.WriteUint32(uint32(len(data)))     // Compressed Size
	cdlw.WriteUint16(uint16(len(name)))     // Name length
	cdlw.WriteUint16(0)                     // Extra Fields
	cdlw.WriteUint16(0)                     // File Comments
	cdlw.WriteUint16(0)                     // Disk Num.
	cdlw.WriteUint16(0)                     // Int. File Attrs.
	cdlw.WriteUint32(0)                     // Ext. File Attrs.
	cdlw.WriteUint32(uint32(lw.Count))      // Offset
	cdlw.WriteString(name)

	lw.WriteUint32(0x04034B50)            // LocalFile Header
	lw.WriteUint16(0x14)                  // Version
	lw.WriteUint16(0)                     // General Purpose Flags
	lw.WriteUint16(method)                // Compression Method
	lw.WriteUint32(0)                     // Modified time/date
	lw.WriteUint32(crc)                   // CRC
	lw.WriteUint32(uint32(len(contents))) // Uncompressed Size
	lw.WriteUint32(uint32(len(mime)))     // Compressed Size
	lw.WriteUint16(uint16(len(name)))     // Name length
	lw.WriteUint16(0)                     // Extra fields
	lw.WriteString(name)
	lw.Write(data)

}

func writeEOCD(lw, cdlw *byteio.StickyLittleEndianWriter) {
	lw.WriteUint32(0x06054B50)         // EOCD
	lw.WriteUint16(0)                  // Disk Num.
	lw.WriteUint16(0)                  // Disk Start
	lw.WriteUint16(3)                  // Number of records
	lw.WriteUint16(3)                  // Total number of records
	lw.WriteUint32(uint32(cdlw.Count)) // Total Central Records length
	lw.WriteUint32(0)                  // Start of Central Records
	lw.WriteUint16(0)
}
