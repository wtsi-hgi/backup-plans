import type { DirectoryWithChildren, Rule } from "./types.js"
import { clearNode } from "./lib/dom.js";
import { br, button, dialog, div, form, h2, html, input, label, option, p, select, span, table, tbody, td, th, thead, tr } from './lib/html.js';
import { linearGradient, svg, title, use } from './lib/svg.js';
import { action, confirm, formatBytes, setAndReturn, stringSort } from "./lib/utils.js";
import { createRule, getTree, removeRule, updateRule, uploadFOFN, user } from "./rpc.js";
import { BackupIBackup, BackupManual, BackupNone } from "./types.js"

const addEditOverlay = (path: string, rule: Rule, load: (path: string) => void) => {
	let validTable: fofnTable | null = null;
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
			evalFOFN(fr, fofnSection, path).then(vt => validTable = vt)

			fr.readAsText(fofn.files![0]);
		}}),
		match = input({ "id": "match", "type": "text", "value": rule.Match, [rule.Match ? "disabled" : "enabled"]: "" }),
		matchLabel = label({ "for": "match" }, "Match"),
		metadata = input({ "id": "metadata", "type": "text", "value": rule.Metadata }),
		metadataSection = div({ "id": "metadataInput" }, [
			label({ "for": "metadata" }, "Metadata"),
			metadata,
			br(),
		]),
		fofnSection = div(),
		overlay = document.body.appendChild(dialog({ "id": "addEdit", "closedby": "any", "close": () => overlay.remove() }, form({
			"submit": (e: SubmitEvent) => {
				e.preventDefault();

				overlay.setAttribute("closedby", "none");
				set.toggleAttribute("disabled", true);
				cancel.toggleAttribute("disabled", true);
				backupType.toggleAttribute("disabled", true);
				fofn.toggleAttribute("disabled", true);

				if (validTable) {
					uploadFOFN(path, backupType.value, metadata.value, validTable.files) // TODO: i dont think path is right here
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
					return;
				}

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
			matchLabel, match, br(),
			label({"for": "fofn", "class": "fofn"}, "FOFN"), fofn, br(),
			label({ "for": "backupType" }, "Backup Type"), backupType, br(),
			metadataSection,
			fofnSection,
			set,
			cancel,
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

function evalFOFN(fr: FileReader, fofnSection : HTMLElement, dir : string): Promise<fofnTable> {
	return new Promise((resolve, reject) => {
		fr.onload = () => {
			if (fr.result === null) {
				reject(new Error("FOFN file is empty"));
				return;
			}

			// Initialise HTML elements
			const title = document.createElement('h2');
			title.textContent = 'Uploading FOFN files:';
			const validLines = document.createDocumentFragment();
			const invalidLines = document.createDocumentFragment();
			const invalidHeader = document.createElement('h3');
			invalidHeader.textContent = 'Invalid filepaths:';

			const validTable = new fofnTable(validLines, 'To Claim:', 'Rules:');
			const invalidTable = new fofnTable(invalidLines, 'Reason:', 'Lines:');

			// Parse lines
			const lines = (fr.result as string).trim().split('\n');
			let count = 0; // Track total valid files
			const seen = new Set<string>(); // Track duplicates

			const fofn = new Map<string, string[]>();

			for (let line of lines) {
				// Filter out comments
				if (line.includes('#')) {
					const index = line.indexOf('#');
					line = line.substring(0, index).trim();
				};

				// Filter empty lines
				if (line === "") {
					continue;
				};

				// Check errors
				if (seen.has(line)) {
					invalidTable.addLine("Duplicate ", line);
					continue;
				};
				seen.add(line);

				// Filter out wildcards
				if (line.includes('*')) {
					invalidTable.addLine("Filename cannot contain * ", line);
					continue;
				};

				// Check dir exists within current dir
				if (!line.startsWith(dir)) {
					invalidTable.addLine("Outside of current dir ", line);
					continue;
				}

				const dirToClaim = line.substring(0, line.lastIndexOf("/") + 1);
				(fofn.get(dirToClaim) ?? setAndReturn(fofn, dirToClaim, [])).push(line.substring(line.lastIndexOf("/")+1));
			}



			const ps: Promise<void>[] = [];
			for (const [dir, rules] of fofn.entries()) {
				ps.push(getTree(dir).then(data => {
					let toClaim = false;
					if (data.ClaimedBy == user) {
						
					} else if (data.CanClaim) {
						toClaim = true;
					} else {
						invalidTable.addLine("Cannot claim directory ", dir)
						return
					}
					const s = new Set<string>(Array.from(Object.values(data.Rules[dir] ?? [])).map(r => r.Match));

					console.log(s)

					for (const filename of rules) {
						validTable.addLine(dir, filename, toClaim, s.has(filename));
					}
				}).catch(() => {
					for (const filename of rules) {
						invalidTable.addLine("Unable to access directory ", dir+filename);
					}
				}))
			}

			Promise.all(ps).then(() => {
				if (validTable.fileCount > 100) {
					// Enforce file limit	
					const p = document.createElement("p");
					p.style.color = 'red';
					p.textContent = 'Number of files cannot exceed 100.';	

					clearNode(fofnSection, p);
					reject();

					return
				}
				clearNode(fofnSection, [title, validLines, invalidHeader, invalidLines])
				resolve(validTable);
			})

		};
	})
};

interface RowData {
    element: HTMLParagraphElement;
	valuesContainer: HTMLDivElement;
    values: string[];
}

class fofnTable {
	fragment: DocumentFragment;
	existsMap: Map<string, RowData>;
	files: string[];
	fileCount = 0;

	constructor(fragment: DocumentFragment, keyHeader: string, valHeader: string) {
		this.fragment = fragment;
		this.existsMap = new Map();
		this.files = [];

		const header = p();
		header.classList.add('valid', 'header-row');
		header.append(span(keyHeader),span(),span(valHeader));
		this.fragment.appendChild(header);
	}

	addLine(key: string, value: string, toClaim: boolean = false, overwrite: boolean = false) {
		this.fileCount++;

		if (this.existsMap.has(key)) {
			const rowData = this.existsMap.get(key)!;
			rowData.values.push(value);
			const val = span(value);
			if (overwrite) {
				val.classList.add('overwrite')
			}
			rowData.valuesContainer.appendChild(val);
			this.files.push(key + value);

            return;
		}

		
		const keyCol = span(key);
		if (toClaim) {
			keyCol.classList.add('claim');
		}
		const rightColumn = span(value);
		if (overwrite) {
			rightColumn.classList.add('overwrite')
		}
		const seperator = span(':');
		const rightColContainer = div();
		rightColContainer.classList.add('c3-container');
		rightColContainer.appendChild(rightColumn);

		const row = p();
		row.append(keyCol, seperator, rightColContainer);
		row.classList.add('valid');

		this.existsMap.set(key,{
			element: row,
			valuesContainer: rightColContainer,
			values: [value]
		});

		this.files.push(key + value);
		this.fragment.appendChild(row);
	}
}

// interface RowData {
// 	row: HTMLTableRowElement;
// 	valuesCell: HTMLTableCellElement;
// 	values: string[];
// }

// class FofnTable {
// 	table: HTMLTableElement;
// 	files: string[];
// 	rows: Map<string, RowData>;
	
// 	constructor(fragment: DocumentFragment, keyHeader: string, valueHeader: string) {
// 		this.table = this.createTable(keyHeader, valueHeader);
// 		fragment.appendChild(this.table);
// 	}

// 	createTable(keyHeader : string, valueHeader : string): HTMLTableElement {
// 		return table({}, [
// 			thead(tr([
// 				th(keyHeader),
// 				td(),
// 				th(valueHeader)
// 			])),
// 			tbody([
// 				tr([
// 					td(), // keys?
// 					td(':'),
// 					td() // values?
// 				])
// 			])
// 		])
// 	}

// 	addLine(key: string, value: string, claim: boolean = false, overwrite: boolean = false) {
// 		if (this.rows.has(key)) {

// 		}

// 	}