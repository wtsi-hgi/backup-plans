import type { ReadSummary, Rule } from "./types.js"
import { clearNode } from "./lib/dom.js";
import { br, button, dialog, div, input, label, option, select, table, tbody, td, th, thead, tr } from './lib/html.js';
import { svg, title, use } from './lib/svg.js';
import { createRule, removeRule, updateRule, user } from "./rpc.js";

const actions = ["No Backup", "Temp Backup", "IBackup", "Manual Backup"],
	action = (backupType: number) => actions[backupType] ?? "Unknown",
	addEditOverlay = (path: string, rule: Rule, load: (path: string) => void) => {
		const backupType = select({ "id": "backupType" }, [
			option({ "value": "backup", [rule.BackupType === 2 ? "selected" : "unselected"]: "" }, "Backup"),
			option({ "value": "tempbackup", [rule.BackupType === 1 ? "selected" : "unselected"]: "" }, "Temp Backup"),
			option({ "value": "manualbackup", [rule.BackupType === 3 ? "selected" : "unselected"]: "" }, "Manual Backup"),
			option({ "value": "nobackup", [rule.BackupType === 0 ? "selected" : "unselected"]: "" }, "No Backup")
		]),
			match = input({ "id": "match", "type": "text", "value": rule.Match, [rule.Match ? "disabled" : "enabled"]: "" }),
			overlay = document.body.appendChild(dialog({ "closedby": "any", "close": () => overlay.remove() }, [
				label({ "for": "match" }, "Match"), match, br(),
				label({ "for": "backupType" }, "Backup Type"), backupType, br(),
				button({
					"click": () => (rule.Match ? updateRule : createRule)(path, backupType.value, rule.Match || match.value || "*")
						.then(() => {
							load(path);
							overlay.remove();
						})
						.catch((e: Error) => alert("Error: " + e.message))
				}, rule.Match ? "Update" : "Add"),
				button({ "click": () => overlay.close() }, "Cancel"),
			]));

		overlay.showModal();
	},
	base = div();

export default Object.assign(base, {
	"update": (path: string, data: ReadSummary, load: (path: string) => void) => {
		clearNode(base, [
			data.rules.ClaimedBy && data.rules.ClaimedBy == user ? button({
				"click": () => addEditOverlay(path, {
					"BackupType": 2,
					"Metadata": "",
					"ReviewDate": 0,
					"RemoveDate": 0,
					"Match": "",
					"Frequency": 7
				}, load),
			}, "Add Rule") : [],
			data.rules.ClaimedBy ? table({ "id": "rules" }, [
				thead(tr([th("Match"), th("Action"), th()])),
				tbody(Object.values(data.rules.Rules).map(rule => tr([
					td(rule.Match),
					td(action(rule.BackupType)),
					td([
						button({ "click": () => addEditOverlay(path, rule, load) }, svg([
							title("Edit Rule"),
							use({ "href": "#edit" })
						])),
						button({
							"click": () => {
								if (confirm("Are you sure?")) {
									removeRule(path, rule.Match)
										.then(() => load(path));
								}
							}
						}, svg([
							title("Remove Rule"),
							use({ "href": "#remove" })
						]))
					])
				])))
			]) : []
		]);
	}
});