import type { ReadSummary } from "./types.js"
import { br, button, dialog, input, label, option, select, table, tbody, td, th, thead, tr } from './lib/html.js';
import { createRule, removeRule, user } from "./rpc.js";

const actions = ["No Backup", "Temp Backup", "IBackup", "Manual Backup"],
	action = (backupType: number) => actions[backupType] ?? "Unknown";

export const Rules = (path: string, data: ReadSummary, loadTree: (path: string) => void) => {
	return [
		data.rules.ClaimedBy && data.rules.ClaimedBy == user ? button({
			"click": () => {
				const backupType = select({ "id": "backupType" }, [
					option({ "value": "backup" }, "Backup"),
					option({ "value": "tempbackup" }, "Temp Backup"),
					option({ "value": "manualbackup" }, "Manual Backup"),
					option({ "value": "nobackup" }, "No Backup")
				]),
					match = input({ "id": "match", "type": "text" }),
					overlay = document.body.appendChild(dialog({ "open": "open" }, [
						label({ "for": "backupType" }, "Backup Type"), backupType, br(),
						label({ "for": "match" }, "Match"), match, br(),
						button({
							"click": () => createRule(path, backupType.value, match.value)
								.then(() => {
									loadTree(path);
									overlay.remove();
								})
								.catch((e: Error) => alert("Error: " + e.message))
						}, "Add"),
						button({ "click": () => overlay.remove() }, "Cancel"),
					]));
			},
		}, "Add Rule") : [],
		data.rules.ClaimedBy ? table([
			thead(tr([th("Action"), th("Match"), th()])),
			tbody(Object.values(data.rules.Rules).map(rule => tr([
				td(action(rule.BackupType)),
				td(rule.Match),
				td(button({
					"click": () => {
						if (confirm("Are you sure?")) {
							removeRule(path, rule.Match)
								.then(() => loadTree(path));
						}
					}
				}, "X"))
			])))
		]) : []
	];
}