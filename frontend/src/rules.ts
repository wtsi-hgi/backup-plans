import type { ReadSummary, Rule } from "./types.js"
import { br, button, dialog, div, input, label, option, select, table, tbody, td, th, thead, tr } from './lib/html.js';
import { createRule, removeRule, updateRule, user } from "./rpc.js";
import { clearNode } from "./lib/dom.js";

const actions = ["No Backup", "Temp Backup", "IBackup", "Manual Backup"],
	action = (backupType: number) => actions[backupType] ?? "Unknown",
	addEditOverlay = (path: string, rule: Rule, load: (path: string) => void) => {
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
					"click": () => (rule.Match ? updateRule : createRule)(path, backupType.value, match.value)
						.then(() => {
							load(path);
							overlay.remove();
						})
						.catch((e: Error) => alert("Error: " + e.message))
				}, rule.Match ? "Update" : "Add"),
				button({ "click": () => overlay.remove() }, "Cancel"),
			]));
	},
	base = div();

export default Object.assign(base, {
	"update": (path: string, data: ReadSummary, load: (path: string) => void) => {
		clearNode(base, [
			data.rules.ClaimedBy && data.rules.ClaimedBy == user ? button({
				"click": () => addEditOverlay(path, {
					"BackupType": 0,
					"Metadata": "",
					"ReviewDate": 0,
					"RemoveDate": 0,
					"Match": "",
					"Frequency": 7
				}, load),
			}, "Add Rule") : [],
			data.rules.ClaimedBy ? table([
				thead(tr([th("Action"), th("Match"), th()])),
				tbody(Object.values(data.rules.Rules).map(rule => tr([
					td(action(rule.BackupType)),
					td(rule.Match),
					td([
						button({ "click": () => addEditOverlay(path, rule, load) }, "E"),
						button({
							"click": () => {
								if (confirm("Are you sure?")) {
									removeRule(path, rule.Match)
										.then(() => load(path));
								}
							}
						}, "X")
					])
				])))
			]) : []
		]);
	}
});