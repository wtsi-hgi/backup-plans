import type { BackupType, dirDetails, DirectoryWithChildren, Rule, RuleStats } from "./types.js"
import { clearNode } from "./lib/dom.js";
import { br, button, dialog, div, form, h2, h3, input, label, option, p, select, table, tbody, td, textarea, th, thead, tr } from './lib/html.js';
import { svg, title, use } from './lib/svg.js';
import { action, formatBytes, setAndReturn } from "./lib/utils.js";
import { createRule, getTree, removeRule, setDirDetails, updateRule, uploadFOFN, user, setExists } from "./rpc.js";
import { BackupIBackup, BackupManual, BackupNone } from "./types.js"

const createStuff = (backupType: BackupType, md: string, setText: string, closeFn: () => void) => {
	const metadata = input({ "id": "metadata", "type": "text", "value": md });

	return [
		select({ "id": "backupType" }, [
			option({ "value": "backup", [backupType === BackupIBackup ? "selected" : "unselected"]: "" }, "Backup"),
			option({ "value": "manualbackup", [backupType === BackupManual ? "selected" : "unselected"]: "" }, "Manual Backup"),
			option({ "value": "nobackup", [backupType === BackupNone ? "selected" : "unselected"]: "" }, "No Backup")
		]),
		button({ "value": "set" }, setText),
		button({ "type": "button", "click": closeFn }, "Cancel"),
		metadata,
		div({ "id": "metadataInput" }, [
			label({ "for": "metadata" }, "Metadata"),
			metadata,
			br(),
		])
	] as const;
},
	addEditOverlay = (path: string, rule: Rule, load: (path: string) => void, isFOFN = false) => {
		const [backupType, set, cancel, metadata, metadataSection] = createStuff(rule.BackupType, rule.Metadata, rule.Match ? "Update" : "Add", () => overlay.close()),
			match = input({ "id": "match", "type": "text", "value": rule.Match, [rule.Match ? "disabled" : "enabled"]: "" }),
			disableInputs = () => {
				if (!rule.Match) {
					set.toggleAttribute("disabled", true);
				}

				overlay.setAttribute("closedby", "none");
				set.toggleAttribute("disabled", true);
				cancel.toggleAttribute("disabled", true);
				backupType.toggleAttribute("disabled", true);
			},
			enableInputs = () => {
				if (!rule.Match) {
					set.removeAttribute("disabled");
				}

				overlay.setAttribute("closedby", "any");
				set.removeAttribute("disabled");
				cancel.removeAttribute("disabled");
				backupType.removeAttribute("disabled");
			},
			overlay = document.body.appendChild(dialog({ "id": "addEdit", "closedby": "any", "close": () => overlay.remove() }, form({
				"submit": (e: SubmitEvent) => {
					e.preventDefault();

					disableInputs();

					verifyMetadata(metadata.value)
						.then(valid => {
							if (!valid) {
								enableInputs();

								return;
							}

							return (rule.Match ? updateRule : createRule)(path, backupType.value, rule.Match || match.value || "*", backupType.value === "manualbackup" ? metadata.value : "")
								.then(() => {
									load(path);
									overlay.remove();
								});
						})
						.catch((e: Error) => {
							enableInputs();
							alert("Error: " + e.message);
						});
				}
			}, [
				label({ "for": "match" }, "Match"), match, br(),
				label({ "for": "backupType" }, "Backup Type"), backupType, br(),
				metadataSection,
				set,
				cancel,
			])));

		overlay.showModal();
	},
	addFofnOverlay = (path: string, dirDetails: dirDetails, load: (path: string) => void) => {
		let validTable: FofnTable | null = null;

		const [backupType, set, cancel, metadata, metadataSection] = createStuff(BackupIBackup, "", "Add", () => overlay.close()),
			fofn = input({
				"id": "fofn", "type": "file", "style": "display: none", "change": () => {
					const fr = new FileReader();

					evalFOFN(fr, fofnSection, dirDetails, path).then(vt => validTable = vt);

					fr.readAsText(fofn.files![0]);
				}
			}),
			fofnButton = button({ "type": "button", "click": () => fofn.click() }, "Upload file"),
			fofnPaste = button({
				"type": "button", "click": () => {
					const contents = textarea(),
						pasteOverlay = document.body.appendChild(dialog(
							{ "closedby": "any", "id": "fofnPaste", "close": () => pasteOverlay.remove() },
							[
								contents,
								br(),
								button({
									"type": "button", "click": () => {
										new Promise<FofnTable>((resolve, reject) => parseFofn(contents.value, path, dirDetails, fofnSection, resolve, reject))
											.then(vt => validTable = vt)
											.finally(() => pasteOverlay.close())
									}
								}, "Add"),
								button({ "type": "button", "click": () => pasteOverlay.close() }, "Cancel")
							]
						));

					pasteOverlay.showModal();
				}
			}, "Paste as plain text"),
			matchFofnSection = div({ "id": "matchfofn" }, [
				label({ "for": "fofn" }, "Add FOFN"),
				fofnButton,
				fofn,
				" or ",
				fofnPaste
			]),
			fofnSection = div(),
			disableInputs = () => {
				overlay.setAttribute("closedby", "none");
				set.toggleAttribute("disabled", true);
				cancel.toggleAttribute("disabled", true);
				backupType.toggleAttribute("disabled", true);
				fofnButton.toggleAttribute("disabled", true);
				fofnPaste.toggleAttribute("disabled", true);
				fofn.toggleAttribute("disabled", true);
			},
			enableInputs = () => {
				overlay.setAttribute("closedby", "any");
				set.removeAttribute("disabled");
				cancel.removeAttribute("disabled");
				backupType.removeAttribute("disabled");
				fofnButton.removeAttribute("disabled");
				fofnPaste.removeAttribute("disabled");
				fofn.removeAttribute("disabled");
			},
			overlay = document.body.appendChild(dialog({ "id": "addEdit", "closedby": "any", "close": () => overlay.remove() }, form({
				"submit": (e: SubmitEvent) => {
					e.preventDefault();

					disableInputs();

					verifyMetadata(metadata.value)
						.then(valid => {
							if (!valid) {
								enableInputs();

								return;
							}

							return (validTable ? uploadFOFN(path, backupType.value, metadata.value, validTable.files) : Promise.reject({ "message": "No FOFN selected" }))
								.then(() => {
									load(path);
									overlay.remove();
								});
						})
						.catch((e: Error) => {
							enableInputs();
							alert("Error: " + e.message);
						});
				}
			}, [
				matchFofnSection,
				label({ "for": "backupType" }, "Backup Type"), backupType, br(),
				metadataSection,
				fofnSection,
				set,
				cancel,
			])));

		overlay.showModal();
	},
	addConfirmOverlay = (path: string, rule: Rule, load: (path: string) => void) => {
		const confirm = button({ "value": "confirm" }, "Confirm"),
			cancel = button({ "type": "button", "click": () => overlay.close() }, "Cancel"),
			overlay = document.body.appendChild(dialog({ "id": "addEdit", "closedby": "any", "close": () => overlay.remove() }, form({
				"submit": (e: SubmitEvent) => {
					e.preventDefault();
					confirm.toggleAttribute("disabled", true);
					cancel.toggleAttribute("disabled", true);

					removeRule(path, rule.Match)
						.then(() => {
							load(path);
							overlay.remove();
						}).catch(() => {
							confirm.removeAttribute("disabled");
							cancel.removeAttribute("disabled");
						});
				}
			}, [
				div([
					h2(`Are you sure you wish to remove this rule?`),
					p(` Path: ${path}`),
					p(` Match: ${rule.Match}`)
				]),

				div({ "style": "display: flex; justify-content: center; gap: 0.5rem; margin-top: 0.5rem;" }, [confirm, cancel])
			])));

		overlay.showModal();
	},
	dirDetailOverlay = (path: string, dirDetails: dirDetails, load: (path: string) => void) => {
		const frequency = input({ "id": "frequency", "type": "number", "value": dirDetails.Frequency + "" }),
			review = input({ "id": "review", "type": "date", "value": new Date(dirDetails.ReviewDate * 1000).toISOString().substring(0, 10) }),
			remove = input({ "id": "remove", "type": "date", "value": new Date(dirDetails.RemoveDate * 1000).toISOString().substring(0, 10) }),
			set = button({ "value": "set" }, "Set"),
			cancel = button({ "type": "button", "click": () => overlay.close() }, "Cancel"),
			disableInputs = () => {
				overlay.setAttribute("closedby", "none");
				set.toggleAttribute("disabled", true);
				cancel.toggleAttribute("disabled", true);
				frequency.toggleAttribute("disabled", true);
				review.toggleAttribute("disabled", true);
				remove.toggleAttribute("disabled", true);
			},
			enableInputs = () => {
				overlay.setAttribute("closedby", "any");
				set.removeAttribute("disabled");
				cancel.removeAttribute("disabled");
				frequency.removeAttribute("disabled");
				review.removeAttribute("disabled");
				remove.removeAttribute("disabled");
			},
			overlay = document.body.appendChild(dialog({ "id": "addEdit", "closedby": "any", "close": () => overlay.remove() }, form({
				"submit": (e: SubmitEvent) => {
					e.preventDefault();
					if (review.valueAsDate === null || remove.valueAsDate === null || review.valueAsDate < new Date(Date.now()) || remove.valueAsDate < review.valueAsDate) {
						alert("Please provide valid dates for review and removal.");
						return;
					}
					if (frequency.valueAsNumber < 0 || frequency.valueAsNumber > 100000) {
						alert("Please provide a valid frequency (0-100000).");
						return;
					}
					disableInputs();

					setDirDetails(path, frequency.valueAsNumber, + review.valueAsDate / 1000, + remove.valueAsDate / 1000)
						.then(() => {
							load(path);
							overlay.remove();
						})
						.catch((e: Error) => {
							enableInputs();
							alert("Error: " + e.message);
						});
				}
			}, [
				label({ "for": "frequency" }, "Frequency (days)"), frequency, br(),
				label({ "for": "review" }, "Review Date"), review, br(),
				label({ "for": "remove" }, "Remove Date"), remove, br(),
				set,
				cancel,
			])));

		overlay.showModal();
	},
	addRule = (path: string, load: (path: string) => void) => button({
		"click": () => addEditOverlay(path, {
			"BackupType": BackupIBackup,
			"Metadata": "",
			"Match": ""
		}, load),
	}, "Add Rule"),
	addFOFN = (path: string, load: (path: string) => void, dirDetails: dirDetails) => button({
		"click": () => addFofnOverlay(path, dirDetails, load),
	}, "Add FOFN"),
	addDirDetails = (path: string, load: (path: string) => void, dirDetails: dirDetails) => button({
		"click": () => dirDetailOverlay(path, dirDetails, load),
	}, "Set Directory Details"),
	base = div();

export default Object.assign(base, {
	"update": (path: string, data: DirectoryWithChildren, load: (path: string) => void) => {
		clearNode(base, [
			data.claimedBy ? h2("Rules on this directory") : [],
			data.claimedBy && data.claimedBy == user && !data.rules[path]?.length ? [addRule(path, load), addFOFN(path, load, data), addDirDetails(path, load, data)] : [],
			data.claimedBy && data.rules[path]?.length ? table({ "id": "rules", "class": "summary" }, [
				thead(tr([th("Match"), th("Action"), th("Files"), th("Size"), data.claimedBy === user ? td([addRule(path, load), addFOFN(path, load, data), addDirDetails(path, load, data)]) : []])),
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
							"click": () => addConfirmOverlay(path, rule, load)
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

// verifyMetadata will return true if the metadata setName is valid and exists
function verifyMetadata(metadata: string): Promise<Boolean> {
	if (metadata === "") return Promise.reject({ "message": "Metadata cannot be empty" });

	return setExists(metadata);
}

function evalFOFN(fr: FileReader, fofnSection: HTMLElement, parentDirDetails: dirDetails, dir: string): Promise<FofnTable> {
	return new Promise((resolve, reject) => {
		fr.onload = () => {
			if (fr.result === null) {
				reject(new Error("FOFN file is empty"));

				return;
			}

			return parseFofn(fr.result as string, dir, parentDirDetails, fofnSection, resolve, reject)
		}
	});
}

function parseFofn(result: string, dir: string, parentDirDetails: dirDetails, fofnSection: HTMLElement, resolve: (v: FofnTable) => void, reject: () => void) {
	const validTable = new FofnTable();
	const invalidTable = new FofnTable();

	// Parse lines
	const lines = result.trim().split('\n');

	const seen = new Set<string>(); // Track duplicates

	const fofn = new Map<string, string[]>();

	for (let line of lines) {
		// Filter out comments
		if (line.includes('#')) {
			const index = line.indexOf('#');

			line = line.substring(0, index).trim();
		}

		// Filter empty lines
		if (line === "") {
			continue;
		}

		// Check errors
		if (seen.has(line)) {
			invalidTable.addLine("Duplicate ", line);

			continue;
		}

		seen.add(line);

		// Check dir exists within current dir
		if (!line.startsWith(dir)) {
			invalidTable.addLine("Outside of current dir ", line);

			continue;
		}

		const wci = line.indexOf("*"),
			si = line.substring(0, wci < 0 ? line.length : wci).lastIndexOf("/") + 1,
			dirToClaim = line.substring(0, si);

		(fofn.get(dirToClaim) ?? setAndReturn(fofn, dirToClaim, [])).push(line.substring(si));
	}

	const ps: Promise<void>[] = [];

	for (const [dir, rules] of fofn.entries()) {
		ps.push(getTree(dir).then(data => {
			let toClaim = false;
			if (data.ClaimedBy !== user) {
				if (data.CanClaim) {
					toClaim = true;
				} else {
					invalidTable.addLine("Cannot claim directory ", dir);

					return
				}
			}

			const s = new Set<string>(Array.from(Object.values(data.Rules[dir] ?? [])).map(r => r.Match));

			for (const filename of rules) {
				const diffs: string[] = [];

				if (parentDirDetails.Frequency !== data.Frequency) {
					diffs.push(`Frequency: \n Old: ${data.Frequency} \n New: ${parentDirDetails.Frequency} \n`);
				}
				if (parentDirDetails.ReviewDate !== data.ReviewDate) {
					diffs.push(`Review Date: \n Old: ${new Date(data.ReviewDate * 1000).toLocaleDateString()} \n New: ${new Date(parentDirDetails.ReviewDate * 1000).toLocaleDateString()} \n`);
				}
				if (parentDirDetails.RemoveDate !== data.RemoveDate) {
					diffs.push(`Remove Date: \n Old: ${new Date(data.RemoveDate * 1000).toLocaleDateString()} \n New: ${new Date(parentDirDetails.RemoveDate * 1000).toLocaleDateString()} \n`);
				}

				const overwriteDirDetails = diffs.length > 0;
				const dirDetailText = diffs.join("");

				validTable.addLine(dir, filename, toClaim, s.has(filename), overwriteDirDetails, dirDetailText);
			}
		}).catch(() => {
			for (const filename of rules) {
				invalidTable.addLine("Unable to access directory ", dir + filename);
			}
		}))
	}

	Promise.all(ps).then(() => {
		if (validTable.files.length > 100) {
			clearNode(fofnSection, div({ "class": "tooManyFiles" }, 'Number of files cannot exceed 100.'));
			reject();

			return;
		}

		const title = h2({ "style": "float: left" }, 'Rules from FOFN:');
		const invalidHeader = h3('Invalid filepaths:');

		const key = div({ "id": "fofnKey" }, [
			div("Will claim directory"),
			div("Will overwrite directory detail(s)"),
			div("Will overwrite rule")
		]);

		clearNode(
			fofnSection,
			[
				title,
				validTable.files.length > 0 ? [key, validTable.createTable("Directory", "Match")] : p("No valid filepaths found."),
				invalidTable.files.length > 0 ? [invalidHeader, invalidTable.createTable("Reason", "Line")] : []
			]
		);
		resolve(validTable);
	});
};

type value = {
	value: string;
	overwrite: boolean;
}

type rowData = {
	values: value[];
	claim: boolean;
	overwriteDirDetail: boolean;
	dirDetailText: string;
}

class FofnTable {
	files: string[] = [];
	rows = new Map<string, rowData>();

	createTable(keyHeader: string, valueHeader: string): HTMLTableElement {
		const rows: HTMLElement[] = [];

		for (const [key, z] of this.rows.entries()) {
			const keyAttributes: Record<string, string> = {};
			const classes: string[] = [];

			if (z.claim) classes.push("claim");
			if (z.overwriteDirDetail) {
				classes.push("overwriteDirDetail")
				keyAttributes["title"] = z.dirDetailText;
			}

			keyAttributes["class"] = classes.join(" ");

			rows.push(tr([
				td(keyAttributes, key),
				td(z.values.map(v => div({ "class": v.overwrite ? "overwrite" : "" }, v.value)))
			]));
		}
		return table({}, [
			thead(tr([
				th(keyHeader),
				th(valueHeader)
			])),
			tbody(rows)
		])
	}

	addLine(key: string, value: string, claim: boolean = false, overwrite: boolean = false, overwriteDirDetail: boolean = false, dirDetailText: string = "") {
		const row = this.rows.get(key) ?? setAndReturn(this.rows, key, { values: [], claim: false, overwriteDirDetail: false, dirDetailText });

		row.values.push({ value, overwrite });
		row.claim ||= claim;
		row.overwriteDirDetail ||= overwriteDirDetail;

		this.files.push(key + value);
	}
}