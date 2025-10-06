import type { DirectoryWithChildren, SizeCountTime } from "./types.js";
import { button, table, tbody, td, th, thead, tr } from "./lib/html.js";
import { clearNode } from "./lib/dom.js";
import { formatBytes } from "./lib/utils.js";
import { BackupNone, BackupIBackup, BackupTemp, BackupWarn } from './types.js';
import { claimDir } from "./rpc.js";

const claimedByCell = td(),
	totalCount = td(),
	warnCount = td(),
	nobackupCount = td(),
	tempbackupCount = td(),
	backupCount = td(),
	totalSize = td(),
	warnSize = td(),
	nobackupSize = td(),
	tempbackupSize = td(),
	backupSize = td(),
	base = table({ "class": "summary" }, [
		thead(tr([claimedByCell, th("Total"), th("Warn"), th("No Backup"), th("Temp Backup"), th("Backup")])),
		tbody([
			tr([th("File count"), totalCount, warnCount, nobackupCount, tempbackupCount, backupCount]),
			tr([th("File size"), totalSize, warnSize, nobackupSize, tempbackupSize, backupSize])
		])
	]),
	setSummary = (action: SizeCountTime, count: Element, size: Element) => {
		clearNode(count, action?.count?.toLocaleString() ?? "0");
		clearNode(size, { "title": (action?.size ?? 0).toLocaleString() }, formatBytes(action?.size ?? 0));
	};

export default Object.assign(base, {
	"update": (path: string, data: DirectoryWithChildren, load: (path: string) => void) => {
		console.log(data.claimedBy);
		clearNode(claimedByCell, data.claimedBy || (data.canClaim ? button({ "click": () => claimDir(path).then(() => load(path)) }, "Claim") : []));
		setSummary(data, totalCount, totalSize);
		setSummary(data.actions[BackupWarn], warnCount, warnSize);
		setSummary(data.actions[BackupNone], nobackupCount, nobackupSize);
		setSummary(data.actions[BackupTemp], tempbackupCount, tempbackupSize);
		setSummary(data.actions[BackupIBackup], backupCount, backupSize);
	}
});