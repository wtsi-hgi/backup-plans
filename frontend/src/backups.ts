import { a, button, span, table, tbody, td, th, thead, tr } from "./lib/html"
import { svg, title, use } from "./lib/svg";
import { formatBytes } from "./lib/utils";

export default (
	path: string,
	matchingBackupSize: bigint,
	matchingBackupCount: bigint,
	matchingArchiveSize: bigint,
	matchingArchiveCount: bigint,
	unmatchingBackupSize: bigint,
	unmatchingBackupCount: bigint,
	unmatchingArchiveSize: bigint,
	unmatchingArchiveCount: bigint) => {
	const matchingBackupCountTD = td(matchingBackupCount.toLocaleString()),
		matchingBackupSizeTD = td({ "title": matchingBackupSize.toLocaleString() }, formatBytes(matchingBackupSize)),
		matchingArchiveCountTD = td(matchingArchiveSize.toLocaleString()),
		matchingArchiveSizeTD = td({ "title": matchingArchiveCount.toLocaleString() }, formatBytes(matchingArchiveCount)),
		unmatchedBackupCountTD = td(unmatchingBackupSize.toLocaleString()),
		unmatchedBackupSizeTD = td({ "title": unmatchingBackupCount.toLocaleString() }, formatBytes(unmatchingBackupCount)),
		unmatchedArchiveCountTD = td(unmatchingArchiveSize.toLocaleString()),
		unmatchedArchiveSizeTD = td({ "title": unmatchingArchiveCount.toLocaleString() }, formatBytes(unmatchingArchiveCount));

	return [
		table({ "class": "summary" }, [
			thead([
				tr([
					td(),
					th({ "colspan": "2" }, [
						"Matching",
						span({ "data-tooltip": "Files that were automatically backed-up and are matched by current rules." }, svg(use({ "href": "#helpIcon" }))),
						button({ "click": () => a({ "href": "api/report/files?matching=1&dir=" + path }).click() }, svg([
							title("Download TSV"),
							use({ "href": "#downloadFile" })
						]))
					]),
					th({ "colspan": "2" }, [
						"Unmatched",
						span({ "data-tooltip": "Files that were automatically backed-up but are not matched by current rules." }, svg(use({ "href": "#helpIcon" }))),
						button({ "click": () => a({ "href": "api/report/files?dir=" + path }).click() }, svg([
							title("Download TSV"),
							use({ "href": "#downloadFile" })
						]))
					])
				])
			]),
			tbody([
				tr([
					th([
						"Backed-up",
						span({ "data-tooltip": "Files that were automatically backed-up and still exist locally." }, svg(use({ "href": "#helpIcon" }))),
					]),
					matchingBackupCountTD,
					matchingBackupSizeTD,
					unmatchedBackupCountTD,
					unmatchedBackupSizeTD
				]),
				tr([
					th([
						"Archived",
						span({ "data-tooltip": "Files that were automatically backed-up and do not exist locally." }, svg(use({ "href": "#helpIcon" }))),
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