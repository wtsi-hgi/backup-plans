import { a, button, span, table, tbody, td, th, thead, tr } from "./lib/html.js"
import { svg, title, use } from "./lib/svg.js";
import { formatBytes } from "./lib/utils.js";

export default (
	path: string,
	matchingBackupCount: bigint,
	matchingBackupSize: bigint,
	matchingArchiveCount: bigint,
	matchingArchiveSize: bigint,
	unmatchingBackupCount: bigint,
	unmatchingBackupSize: bigint,
	unmatchingArchiveCount: bigint,
	unmatchingArchiveSize: bigint,
	single = false) => {
	const matchingBackupCountTD = td(matchingBackupCount.toLocaleString()),
		matchingBackupSizeTD = td({ "title": matchingBackupSize.toLocaleString() }, formatBytes(matchingBackupSize)),
		matchingArchiveCountTD = td(matchingArchiveCount.toLocaleString()),
		matchingArchiveSizeTD = td({ "title": matchingArchiveSize.toLocaleString() }, formatBytes(matchingArchiveSize)),
		unmatchedBackupCountTD = td(unmatchingBackupCount.toLocaleString()),
		unmatchedBackupSizeTD = td({ "title": unmatchingBackupSize.toLocaleString() }, formatBytes(unmatchingBackupSize)),
		unmatchedArchiveCountTD = td(unmatchingArchiveCount.toLocaleString()),
		unmatchedArchiveSizeTD = td({ "title": unmatchingArchiveSize.toLocaleString() }, formatBytes(unmatchingArchiveSize));

	return [
		table({ "class": "summary" }, [
			thead([
				tr([
					td(),
					th({ "colspan": "2" }, [
						"Matching",
						span({ "data-tooltip": "Files that were automatically backed up and are matched by current rules." }, svg(use({ "href": "#helpIcon" }))),
						button({ "click": () => a({ "href": "api/report/files?matching=1&dir=" + encodeURIComponent(path) + (single ? "&single=1" : "") }).click() }, svg([
							title("Download CSV"),
							use({ "href": "#downloadFile" })
						]))
					]),
					th({ "colspan": "2" }, [
						"Unmatched",
						span({ "data-tooltip": "Files that were automatically backed up but are not matched by current rules." }, svg(use({ "href": "#helpIcon" }))),
						button({ "click": () => a({ "href": "api/report/files?dir=" + encodeURIComponent(path) + (single ? "&single=1" : "") }).click() }, svg([
							title("Download CSV"),
							use({ "href": "#downloadFile" })
						]))
					])
				])
			]),
			tbody([
				tr([
					th([
						"Backed Up",
						span({ "data-tooltip": "Files that were automatically backed up and still exist locally." }, svg(use({ "href": "#helpIcon" }))),
					]),
					matchingBackupCountTD,
					matchingBackupSizeTD,
					unmatchedBackupCountTD,
					unmatchedBackupSizeTD
				]),
				tr([
					th([
						"Archived",
						span({ "data-tooltip": "Files that were automatically backed up and do not exist locally." }, svg(use({ "href": "#helpIcon" }))),
					]),
					matchingArchiveCountTD,
					matchingArchiveSizeTD,
					unmatchedArchiveCountTD,
					unmatchedArchiveSizeTD
				]),
			])
		]),
		matchingBackupCountTD,
		matchingBackupSizeTD,
		matchingArchiveCountTD,
		matchingArchiveSizeTD,
		unmatchedBackupCountTD,
		unmatchedBackupSizeTD,
		unmatchedArchiveCountTD,
		unmatchedArchiveSizeTD,
		(p: string) => path = p
	] as const;
}