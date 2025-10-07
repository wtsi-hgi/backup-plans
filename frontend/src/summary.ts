import type { DirectoryWithChildren, SizeCountTime } from "./types.js";
import { clearNode } from "./lib/dom.js";
import { br, button, dialog, input, label, table, tbody, td, th, thead, tr } from "./lib/html.js";
import { svg, title, use } from "./lib/svg.js";
import { confirm, formatBytes } from "./lib/utils.js";
import { claimDir, passDirClaim, revokeDirClaim, user } from "./rpc.js";
import { BackupNone, BackupIBackup, BackupTemp, BackupWarn } from './types.js';

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
		setSummary(data, totalCount, totalSize);
		setSummary(data.actions[BackupWarn], warnCount, warnSize);
		setSummary(data.actions[BackupNone], nobackupCount, nobackupSize);
		setSummary(data.actions[BackupTemp], tempbackupCount, tempbackupSize);
		setSummary(data.actions[BackupIBackup], backupCount, backupSize);
	}
});