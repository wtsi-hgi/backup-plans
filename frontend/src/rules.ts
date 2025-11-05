import type { DirectoryWithChildren, Rule } from "./types.js"
import { clearNode } from "./lib/dom.js";
import { br, button, dialog, div, form, h2, input, label, option, p, select, table, tbody, td, th, thead, tr } from './lib/html.js';
import { svg, title, use } from './lib/svg.js';
import { action, confirm, formatBytes, stringSort } from "./lib/utils.js";
import { createRule, removeRule, updateRule, user } from "./rpc.js";
import { BackupIBackup, BackupManual, BackupNone } from "./types.js"

const addEditOverlay = (path: string, rule: Rule, load: (path: string) => void) => {
	const backupType = select({ "id": "backupType" }, [
		option({ "value": "backup", [rule.BackupType === BackupIBackup ? "selected" : "unselected"]: "" }, "Backup"),
		option({ "value": "manualbackup", [rule.BackupType === BackupManual ? "selected" : "unselected"]: "" }, "Manual Backup"),
		option({ "value": "nobackup", [rule.BackupType === BackupNone ? "selected" : "unselected"]: "" }, "No Backup")
	]),
		set = button({"value": "set"}, rule.Match ? "Update" : "Add"),
		cancel = button({ "type": "button", "click": () => overlay.close() }, "Cancel"),
		fofn = input({"id": "fofn", "type": "file", "style": "display: none", "change": () => {
			const fr = new FileReader();
			
			match.toggleAttribute("hidden", true);
			evalFOFN(fr, overlay);

			fr.readAsText(fofn.files![0]);
		}}),
		match = input({ "id": "match", "type": "text", "value": rule.Match, [rule.Match ? "disabled" : "enabled"]: "" }),
		metadata = input({ "id": "metadata", "type": "text", "value": rule.Metadata }),
		metadataSection = div({ "id": "metadataInput" }, [
			label({ "for": "metadata" }, "Metadata"),
			metadata,
			br(),
		]),
		overlay = document.body.appendChild(dialog({ "id": "addEdit", "closedby": "any", "close": () => overlay.remove() }, form({
			"submit": (e: SubmitEvent) => {
				e.preventDefault();

				overlay.setAttribute("closedby", "none");
				set.toggleAttribute("disabled", true);
				cancel.toggleAttribute("disabled", true);
				backupType.toggleAttribute("disabled", true);
				fofn.toggleAttribute("disabled", true);

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
					fofn.removeAttribute("disabled");
					alert("Error: " + e.message);
				});
			}
		}, [
			label({ "for": "match" }, "Match"), match, br(),
			label({"for": "fofn", "class": "fofn"}, "FOFN"), br(),
			label({ "for": "backupType" }, "Backup Type"), backupType, br(),
			metadataSection,
			set,
			cancel,
			fofn
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

function evalFOFN(fr : FileReader, overlay : HTMLDialogElement) {
	fr.onload = () => {
		if (fr.result === null) {
			console.log("fr.result is null")
			return
		}

		// Initialise HTML elements
		const title = document.createElement('h2');
		title.textContent = 'Uploading FOFN files:';
		const validLines = document.createDocumentFragment();
		const invalidLines = document.createDocumentFragment();
		const invalidHeader = document.createElement('h3')
		invalidHeader.textContent = 'Invalid filepaths:';

		const validTable = new fofnTable(validLines, 'To Claim:', 'Rules:');
		const invalidTable = new fofnTable(invalidLines, 'Reason:', 'Rules:');

		// Parse lines
		const lines = (fr.result as string).trim().split('\n');
		var count = 0; // Track total valid files
		const seen = new Set<string>(); // Track duplicates

		for (let line of lines) {
			// Filter out comments
			if (line.includes('#')) {
				const index = line.indexOf('#');
				line = line.substring(0, index).trim();
			};

			// Filter empty lines
			if (line === "") {
				continue
			}

			// Check errors
			if (line.includes('*')) {
				invalidTable.addLine("Filename cannot contain * ", line);
				continue
			} 
			if (seen.has(line)) {
				invalidTable.addLine("Duplicate ", line);
				continue
			}

			count = count + 1;
			seen.add(line)

			const dirToClaim = line.substring(0, line.lastIndexOf("/") + 1);
			validTable.addLine(dirToClaim, line);

			// Enforce file limit	
			if (count > 100) {
				const p = document.createElement("p");
				p.style.color = 'red';
				p.textContent = 'Number of files cannot exceed 100.'
				overlay.appendChild(p)
				return
			}
		};

		overlay.appendChild(title);
		overlay.appendChild(validLines);
		overlay.appendChild(invalidHeader);
		overlay.appendChild(invalidLines);

		console.log("validlines: ", validTable.toJSON())
		console.log("invalidlines: ", invalidTable.toJSON())
	};
};

interface RowData {
    element: HTMLParagraphElement;
	c3container: HTMLDivElement;
    c3vals: string[];
}

class fofnTable {
	fragment: DocumentFragment;
	existsMap: Map<string, RowData>;
	header1: string;
	header3: string;

	constructor(fragment: DocumentFragment, header1: string, header3: string) {
		this.fragment = fragment;
		this.header1 = header1;
		this.header3 = header3;
		this.existsMap = new Map();

		const header = document.createElement('p');
		header.classList.add('valid', 'header-row');

		const col1 = document.createElement('span');
		col1.textContent = header1;

		const col2 = document.createElement('span'); // colon column

		const col3 = document.createElement('span');
		col3.textContent = header3;

		header.appendChild(col1);
		header.appendChild(col2);
		header.appendChild(col3);
		this.fragment.appendChild(header);
	}

	addLine(c1: string, c3: string) {
		if (this.existsMap.has(c1)) {
			const rowData = this.existsMap.get(c1)!;
			rowData.c3vals.push(c3);
			const col3 = document.createElement('span');
			col3.textContent = c3;
			rowData.c3container.appendChild(col3);

            return;
		}

		const p = document.createElement("p");
		const col1 = document.createElement('span');
		col1.textContent = c1;
		const col3 = document.createElement('span');
		col3.textContent = c3;
		const spanColon = document.createElement('span');
		spanColon.textContent = ' : ';
		const c3container = document.createElement('div');
		c3container.classList.add('c3-container');
		c3container.appendChild(col3);

		p.appendChild(col1);
		p.appendChild(spanColon);
		p.appendChild(c3container);
		p.classList.add('valid');
		this.existsMap.set(c1,{
			element: p,
			c3container: c3container,
			c3vals: [c3]
		});
		this.fragment.appendChild(p);
	}

	toJSON() {
		const output = {} as Record<string, any>;
		for (let [c1, rowData] of this.existsMap) {
			output[c1] = rowData.c3vals;
		}
		return output;
	}
}