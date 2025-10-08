import { BackupIBackup, type DirectoryWithChildren, type Rule } from "./types.js"
import { clearNode } from "./lib/dom.js";
import { br, button, caption, dialog, div, h2, input, label, option, select, table, tbody, td, th, thead, tr } from './lib/html.js';
import { svg, title, use } from './lib/svg.js';
import { confirm, formatBytes } from "./lib/utils.js";
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
			set = button({
				"click": () => {
					overlay.setAttribute("closedby", "none");
					set.toggleAttribute("disabled", true);
					cancel.toggleAttribute("disabled", true);
					backupType.toggleAttribute("disabled", true);

					(rule.Match ? updateRule : createRule)(path, backupType.value, rule.Match || match.value || "*")
						.then(() => {
							load(path);
							overlay.remove();
						})
						.catch((e: Error) => {
							overlay.setAttribute("closedby", "any");
							set.removeAttribute("disabled");
							cancel.removeAttribute("disabled");
							backupType.removeAttribute("disabled");
							alert("Error: " + e.message);
						});
				}
			}, rule.Match ? "Update" : "Add"),
			cancel = button({ "click": () => overlay.close() }, "Cancel"),
			match = input({ "id": "match", "type": "text", "value": rule.Match, [rule.Match ? "disabled" : "enabled"]: "" }),
			overlay = document.body.appendChild(dialog({ "closedby": "any", "close": () => overlay.remove() }, [
				label({ "for": "match" }, "Match"), match, br(),
				label({ "for": "backupType" }, "Backup Type"), backupType, br(),
				set,
				cancel
			]));

		overlay.showModal();
	},
	addRule = (path: string, load: (path: string) => void) => button({
		"click": () => addEditOverlay(path, {
			"BackupType": BackupIBackup,
			"Metadata": "",
			"ReviewDate": 0,
			"RemoveDate": 0,
			"Match": "",
			"Frequency": 7
		}, load),
	}, "Add Rule"),
	base = div();

export default Object.assign(base, {
	"update": (path: string, data: DirectoryWithChildren, load: (path: string) => void) => {
		clearNode(base, [
			data.claimedBy ? h2("Rules on this directory") : [],
			data.claimedBy && data.claimedBy == user && !data.rules[path]?.length ? addRule(path, load) : [],
			data.claimedBy && data.rules[path]?.length ? table({ "id": "rules", "class": "summary" }, [
				thead(tr([th("Match"), th("Action"), th("Files"), th("Size"), data.claimedBy === user ? td(addRule(path, load)) : []])),
				tbody(Object.values(data.rules[path] ?? []).map(rule => tr([
					td(rule.Match),
					td(action(rule.BackupType)),
					td(rule.count.toLocaleString()),
					td({ "title": rule.size.toLocaleString() }, formatBytes(rule.size)),
					data.claimedBy === user ? td([
						button({
							"class": "actionButton",
							"click": () => addEditOverlay(path, rule, load)
						}, svg([
							title("Edit Rule"),
							use({ "href": "#edit" })
						])),
						button({
							"class": "actionButton",
							"click": () => confirm("Are you sure you wish to remove this rule?", () => removeRule(path, rule.Match).then(() => load(path)))
						}, svg([
							title("Remove Rule"),
							use({ "href": "#remove" })
						]))
					]) : []
				])))
			]) : [],
			Object.entries(data.rules).some(([dir]) => dir && dir !== path) ? h2("Rules affecting this directory") : [],
			div({ "class": "affectingRules" }, Object.entries(data.rules).map(([dir, rules]) => dir && dir !== path ? table({ "class": "prettyTable" }, [
				caption({ "align": "top" }, button({ "click": () => load(dir) }, dir)),
				thead([th("Match"), th("Action"), th("Files"), th("Size")]),
				tbody(rules.map(rule => tr([
					td(rule.Match),
					td(action(rule.BackupType)),
					td(rule.count.toLocaleString()),
					td({ "title": rule.size.toLocaleString() }, formatBytes(rule.size))
				])))
			]) : []))
		]);
	}
});