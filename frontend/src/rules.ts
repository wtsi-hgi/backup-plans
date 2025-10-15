import { BackupIBackup, type DirectoryWithChildren, type Rule } from "./types.js"
import { clearNode } from "./lib/dom.js";
import { br, button, dialog, div, form, h2, input, label, meta, option, select, table, tbody, td, th, thead, tr } from './lib/html.js';
import { svg, title, use } from './lib/svg.js';
import { action, confirm, formatBytes } from "./lib/utils.js";
import { createRule, removeRule, updateRule, user } from "./rpc.js";

const addEditOverlay = (path: string, rule: Rule, load: (path: string) => void) => {
	const backupType = select({ "id": "backupType" }, [
		option({ "value": "backup", [rule.BackupType === 2 ? "selected" : "unselected"]: "" }, "Backup"),
		option({ "value": "tempbackup", [rule.BackupType === 1 ? "selected" : "unselected"]: "" }, "Temp Backup"),
		option({ "value": "manualbackup", [rule.BackupType === 3 ? "selected" : "unselected"]: "" }, "Manual Backup"),
		option({ "value": "nobackup", [rule.BackupType === 0 ? "selected" : "unselected"]: "" }, "No Backup")
	]),
		set = button(rule.Match ? "Update" : "Add"),
		cancel = button({ "type": "button", "click": () => overlay.close() }, "Cancel"),
		match = input({ "id": "match", "type": "text", "value": rule.Match, [rule.Match ? "disabled" : "enabled"]: "" }),
		metadata = input({ "id": "metadata", "type": "text", "value": rule.Metadata }),
		metadataSection = div({ "id": "metadataInput" }, [
			label({ "for": "metadata" }, "Metadata"),
			metadata,
			br(),
		]),
		overlay = document.body.appendChild(dialog({ "id": "addEdit", "closedby": "any", "close": () => overlay.remove() }, form({
			"submit": (e: Event) => {
				e.preventDefault();
				overlay.setAttribute("closedby", "none");
				set.toggleAttribute("disabled", true);
				cancel.toggleAttribute("disabled", true);
				backupType.toggleAttribute("disabled", true);

				(rule.Match ? updateRule : createRule)(path, backupType.value, rule.Match || match.value || "*", backupType.value === "manualbackup" ? metadata.value : "")
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
		}, [
			label({ "for": "match" }, "Match"), match, br(),
			label({ "for": "backupType" }, "Backup Type"), backupType, br(),
			metadataSection,
			set,
			cancel
		])));

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
			]) : []
		]);
	}
});