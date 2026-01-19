import type { Children, PropertiesOrChildren } from './lib/dom.js';
import { amendNode } from './lib/dom.js';
import { formatBytes } from './lib/utils.js';
import { contentData, contentMetaInsert, cdfhMetaInsert, ods } from './odf_data.js';

type SSRow = {
	Programme: string;
	Faculty: string;
	Path: string;
	Group: string;
	Status: "r" | "g" | "b" | "a";
	Unplanned: bigint;
	NoBackup: bigint;
	Backup: bigint;
	ManualBackup: bigint;
}

type SummaryRow = {
	Programme: string;
	UnplannedC: bigint;
	UnplannedS: bigint;
	UnplannedF: Number;
	NoBackupC: bigint;
	NoBackupS: bigint;
	BackupC: bigint;
	BackupS: bigint;
	ManualC: bigint;
	ManualS: bigint;
}

const eocdOffset = 0x6,
	generateCRC = (() => {
		const table = new Uint32Array(256);

		let crc = 1 | 0;

		for (var n = 0; n < 256; n++) {
			crc = n;

			for (var k = 0; k < 8; k++) {
				crc = ((crc & 1) ? (0xedb88320 ^ (crc >>> 1)) : (crc >>> 1));
			}

			table[n] = crc;
		}

		return (bytes: Uint8Array) => {
			let crc = 0 ^ (-1);

			for (const b of bytes) {
				crc = (crc >>> 8) ^ table[(crc ^ b) & 0xff];
			}

			return (crc ^ (-1)) >>> 0;
		}
	})(),
	setUint32 = (offset: number, value: number) => ods.set([value & 255, (value >> 8) & 255, (value >> 16) & 255, (value >> 24) & 255], offset),
	xmlns = "http://www.w3.org/2000/xmlns/";

export default (data: SSRow[], summaryData: SummaryRow[]) => {
	const officeNS = "urn:oasis:names:tc:opendocument:xmlns:office:1.0",
		tableNS = "urn:oasis:names:tc:opendocument:xmlns:table:1.0",
		textNS = "urn:oasis:names:tc:opendocument:xmlns:text:1.0",
		styleNS = "urn:oasis:names:tc:opendocument:xmlns:style:1.0",
		foNS = "urn:oasis:names:tc:opendocument:xmlns:xsl-fo-compatible:1.0",
		content = document.implementation.createDocument(officeNS, "document-content"),
		de = content.documentElement,
		tags = (ns: string) => new Proxy({}, { "get": (_, element: string) => (props: PropertiesOrChildren = {}, children?: Children) => amendNode(content.createElementNS(ns, element), props, children) }) as Record<string, (props?: PropertiesOrChildren, children?: Children) => Element>,
		{ "automatic-styles": automaticStyles, body, spreadsheet } = tags(officeNS),
		{ "database-range": databaseRange, "database-ranges": databaseRanges, "named-expressions": namedExpressions, table, "table-column": tableCol, "table-row": tableRow, "table-cell": tableCell } = tags(tableNS),
		{ p } = tags(textNS),
		{ style, "table-cell-properties": tableCellProperties } = tags(styleNS),
		statuses = {
			"r": "No backup in 6 weeks",
			"g": "Backup within 2 weeks",
			"b": "No files to backup",
			"a": "No backup in 2 weeks"
		};

	de.setAttributeNS(xmlns, "xmlns:office", officeNS);
	de.setAttributeNS(xmlns, "xmlns:table", tableNS);
	de.setAttributeNS(xmlns, "xmlns:text", textNS);
	de.setAttributeNS(xmlns, "xmlns:style", styleNS);
	de.setAttributeNS(xmlns, "xmlns:fo", foNS);

	amendNode(de, { "office:version": "1.4" }, [
		automaticStyles([
			style({ "style:name": "bytes", "style:family": "table-cell", "style:parent-style-name": "Default", "style:data-style-name": "Bytes" }),
			[
				["r", "#ff0000"],
				["g", "#00aa00"],
				["b", "#44aaf7"],
				["a", "#ffaa00"]
			].map(s => style({ "style:name": s[0], "style:family": "table-cell", "style:parent-style-name": "Default" }, tableCellProperties({ "fo:background-color": s[1] })))
		]),
		body(spreadsheet([
			table({ "table:name": "Backup Plans" }, [
				tableCol({ "table:number-columns-repeated": "5" }),
				tableCol({ "table:number-columns-repeated": "4", "table:default-cell-style-name": "bytes" }),
				tableRow([
					tableCell({ "office:value-type": "string" }, p("Programme")),
					tableCell({ "office:value-type": "string" }, p("Faculty")),
					tableCell({ "office:value-type": "string" }, p("Path")),
					tableCell({ "office:value-type": "string" }, p("Group")),
					tableCell({ "office:value-type": "string" }, p("Status")),
					tableCell({ "office:value-type": "string" }, p("Unplanned")),
					tableCell({ "office:value-type": "string" }, p("NoBackup")),
					tableCell({ "office:value-type": "string" }, p("Backup")),
					tableCell({ "office:value-type": "string" }, p("Manual Backup")),
				]),
				data.map(row => tableRow([
					tableCell({ "office:value-type": "string" }, p(row.Programme)),
					tableCell({ "office:value-type": "string" }, p(row.Faculty)),
					tableCell({ "office:value-type": "string" }, p(row.Path)),
					tableCell({ "office:value-type": "string" }, p(row.Group)),
					tableCell({ "office:value-type": "string", "table:style-name": row.Status }, p(statuses[row.Status])),
					tableCell({ "office:value": row.Unplanned + "", "office:value-type": "float" }, p(formatBytes(row.Unplanned))),
					tableCell({ "office:value": row.NoBackup + "", "office:value-type": "float" }, p(formatBytes(row.NoBackup))),
					tableCell({ "office:value": row.Backup + "", "office:value-type": "float" }, p(formatBytes(row.Backup))),
					tableCell({ "office:value": row.ManualBackup + "", "office:value-type": "float" }, p(formatBytes(row.ManualBackup)))
				])),
				databaseRanges(databaseRange({ "table:name": "__Anonymous_Sheet_DB__0", "table:target-range-address": "'Backup Plans'.A1:'Backup Plans'.E" + (data.length + 1), "table:display-filter-buttons": "true" }))
			]),
			table({ "table:name": "Summary" }, [
				tableCol({ "table:number-columns-repeated": "2" }),
				tableCol({ "table:number-columns-repeated": "1", "table:default-cell-style-name": "bytes" }),
				tableCol({ "table:number-columns-repeated": "2" }),
				tableCol({ "table:number-columns-repeated": "1", "table:default-cell-style-name": "bytes" }),
				tableCol({ "table:number-columns-repeated": "1" }),
				tableCol({ "table:number-columns-repeated": "1", "table:default-cell-style-name": "bytes" }),
				tableCol({ "table:number-columns-repeated": "1" }),
				tableCol({ "table:number-columns-repeated": "1", "table:default-cell-style-name": "bytes" }),
				tableRow([
					tableCell({ "office:value-type": "string" }, p("BOM")),
					tableCell({ "office:value-type": "string" }, p("Unplanned Count")),
					tableCell({ "office:value-type": "string" }, p("Unplanned Size")),
					tableCell({ "office:value-type": "string" }, p("Unplanned Fraction")),
					tableCell({ "office:value-type": "string" }, p("NoBackup Count")),
					tableCell({ "office:value-type": "string" }, p("NoBackup Size")),
					tableCell({ "office:value-type": "string" }, p("Backup Count")),
					tableCell({ "office:value-type": "string" }, p("Backup Size")),
					tableCell({ "office:value-type": "string" }, p("Manual Backup Count")),
					tableCell({ "office:value-type": "string" }, p("Manual Backup Size")),
				]),
				summaryData.map(row => tableRow([
					tableCell({ "office:value-type": "string" }, p(row.Programme)),
					tableCell({ "office:value": row.UnplannedC + "", "office:value-type": "float" }, p(String(row.UnplannedC))),
					tableCell({ "office:value": row.UnplannedS + "", "office:value-type": "float" }, p(formatBytes(row.UnplannedS))),
					tableCell({ "office:value": row.UnplannedF + "", "office:value-type": "float" }, p(String(row.UnplannedF))),
					tableCell({ "office:value": row.NoBackupC + "", "office:value-type": "float" }, p(String(row.NoBackupC))),
					tableCell({ "office:value": row.NoBackupS + "", "office:value-type": "float" }, p(formatBytes(row.NoBackupS))),
					tableCell({ "office:value": row.BackupC + "", "office:value-type": "float" }, p(String(row.BackupC))),
					tableCell({ "office:value": row.BackupS + "", "office:value-type": "float" }, p(formatBytes(row.BackupS))),
					tableCell({ "office:value": row.ManualC + "", "office:value-type": "float" }, p(String(row.ManualC))),
					tableCell({ "office:value": row.ManualS + "", "office:value-type": "float" }, p(formatBytes(row.ManualS)))
				]))
			]),
			namedExpressions(),
		]))
	]);

	const contentBytes = Uint8Array.from(("<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?>\n" + new XMLSerializer().serializeToString(content)).split('').map(c => c.charCodeAt(0))),
		odsBytes = new Uint8Array(contentBytes.length + ods.length),
		crcV = generateCRC(contentBytes);

	setUint32(contentMetaInsert, crcV);
	setUint32(contentMetaInsert + 4, contentBytes.length);
	setUint32(contentMetaInsert + 8, contentBytes.length);
	setUint32(cdfhMetaInsert, crcV);
	setUint32(cdfhMetaInsert + 4, contentBytes.length);
	setUint32(cdfhMetaInsert + 8, contentBytes.length);

	setUint32(ods.length - eocdOffset, contentData + contentBytes.length);

	odsBytes.set(ods.slice(0, contentData), 0);
	odsBytes.set(contentBytes, contentData);
	odsBytes.set(ods.slice(contentData), contentData + contentBytes.length);

	return odsBytes;
}