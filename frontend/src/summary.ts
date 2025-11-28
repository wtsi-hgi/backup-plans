import type { DirectoryWithChildren, SizeCountTime, SizeCount } from "./types.js";
import { clearNode } from "./lib/dom.js";
import { br, button, dialog, input, label, table, tbody, td, th, thead, tr } from "./lib/html.js";
import { svg, title, use } from "./lib/svg.js";
import { confirm, formatBytes } from "./lib/utils.js";
import { claimDir, passDirClaim, revokeDirClaim, user } from "./rpc.js";
import { BackupNone, BackupIBackup, BackupWarn, ManualBackupTypes } from './consts.js';

const claimedByCell = td(),
	totalCount = td(),
	warnCount = td(),
	nobackupCount = td(),
	backupCount = td(),
	manualBackupCount = td(),
	totalSize = td(),
	warnSize = td(),
	nobackupSize = td(),
	backupSize = td(),
	manualBackupSize = td(),
	base = table({ "class": "summary" }, [
		thead(tr([claimedByCell, th("Total"), th("Unplanned"), th("No Backup"), th("Backup"), th("Manual Backup")])),
		tbody([
			tr([th("File count"), totalCount, warnCount, nobackupCount, backupCount, manualBackupCount]),
			tr([th("File size"), totalSize, warnSize, nobackupSize, backupSize, manualBackupSize])
		])
	]),
	setSummary = (action: SizeCountTime, count: Element, size: Element) => {
		clearNode(count, action?.count?.toLocaleString() ?? "0");
		clearNode(size, { "title": (action?.size ?? 0).toLocaleString() }, formatBytes(action?.size ?? 0));
	};

export default Object.assign(base, {
	"update": (path: string, data: DirectoryWithChildren, load: (path: string) => void) => {
		clearNode(claimedByCell, data.claimedBy ?
			[data.claimedBy, data.claimedBy === user ? data.rules[path]?.length ?
				button({
					"class": "actionButton",
					"click": () => {
						const passTo = input({ "id": "passTo", "placeholder": "Username" }),
							set = button({
								"click": () => {
									overlay.setAttribute("closedby", "none");
									set.toggleAttribute("disabled", true);
									cancel.toggleAttribute("disabled", true);
									passTo.toggleAttribute("disabled", true);

									passDirClaim(path, passTo.value)
										.then(() => {
											load(path);
											overlay.remove();
										})
										.catch((e: Error) => {
											overlay.setAttribute("closedby", "any");
											set.removeAttribute("disabled");
											cancel.removeAttribute("disabled");
											passTo.removeAttribute("disabled");
											alert("Error: " + e.message);
										});
								}
							}, "Pass"),
							cancel = button({ "click": () => overlay.close() }, "Cancel"),
							overlay = document.body.appendChild(dialog({ "closedby": "any", "close": () => overlay.remove() }, [
								label({ "for": "passTo" }, "Pass To"), passTo,
								br(),
								set,
								cancel
							]));

						overlay.showModal();
					}
				}, svg([
					title("Pass Claim"),
					use({ "href": "#edit" })
				]))
				: button({
					"class": "actionButton",
					"click": () => confirm("Are you sure you wish to remove your claim on this directory?", () => revokeDirClaim(path).then(() => load(path)))
				}, svg([
					title("Revoke Claim"),
					use({ "href": "#remove" })
				])) : []]
			: data.canClaim ? button({ "click": () => claimDir(path).then(() => load(path)) }, "Claim") : []);

		const manualActions: SizeCountTime = { count: 0n, size: 0n, mtime: 0 };
		ManualBackupTypes.forEach(backup => {
			manualActions.count += BigInt(data.actions[backup]?.count ?? 0);
			manualActions.size += BigInt(data.actions[backup]?.size ?? 0);
		});

		setSummary(data, totalCount, totalSize);
		setSummary(data.actions[BackupWarn], warnCount, warnSize);
		setSummary(data.actions[BackupNone], nobackupCount, nobackupSize);
		setSummary(data.actions[BackupIBackup], backupCount, backupSize);
		setSummary(manualActions, manualBackupCount, manualBackupSize);
	}
});