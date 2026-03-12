import type { dirDetails, DirectoryWithChildren, Rule, RuleStats } from "./types.js"
import { clearNode } from "./lib/dom.js";
import { br, button, dialog, div, form, h2, h3, input, label, option, p, select, table, tbody, td, textarea, th, thead, tr, span, ul, li } from './lib/html.js';
import { svg, title, use } from './lib/svg.js';
import { action, confirm, formatBytes, secondsInDay, setAndReturn } from "./lib/utils.js";
import { createRule, getTree, removeRule, setDirDetails, updateRule, setExists, user } from "./rpc.js";
import { BackupType, helpText } from "./consts.js"
import { load, registerLoader } from "./load.js";
import { updateClaimStats } from "./claimstats.js";

const createStuff = (backupType: BackupType, md: string, setText: string, closeFn: () => void) => {
	const metadata = input({ "id": "metadata", "type": "text", "value": md }),
		metadataLabel = label({ "for": "metadata", "id": "metadataLabel" }, backupType.metadataLabel()),
		metadataHelpIcon = getHelpIcon(backupType.metadataToolTip()),
		metadataInput = div({ "id": "metadataInput" }, [
			metadataLabel,
			metadataHelpIcon,
			metadata,
			br(),
		]),
		backupSelect = select({
			"id": "backupType", "change": () => {
				const backupType = BackupType.from(backupSelect.value);
				metadataLabel.textContent = backupType.metadataLabel();
				metadataHelpIcon.setAttribute("data-tooltip", backupType.metadataToolTip());
			}
		},
			BackupType.selectable.map(bt => option({ "value": bt.toString(), "selected": +backupType === +bt }, bt.optionLabel()))
		);

	return [
		backupSelect,
		button({ "value": "set" }, setText),
		button({ "type": "button", "click": closeFn }, "Cancel"),
		metadata,
		metadataInput
	] as const;
},
	getHelpIcon = (str: string) => span({ "class": "tooltip", "data-tooltip": str }, svg(use({ "href": "#helpIcon" }))),
	verifyMetadata = (dir: string, backupType: string, metadata: string) => {
		if (!BackupType.from(backupType).isManual()) {
			return Promise.resolve(true);
		}

		if (metadata === "") {
			return Promise.reject({ "message": "Metadata cannot be empty" });
		}

		if (backupType === "manualibackup") {
			return setExists(dir, metadata);
		}

		return Promise.resolve(true);
	},
	addEditOverlay = (path: string, rule: Rule) => {
		const [backupType, set, cancel, metadata, metadataSection] = createStuff(rule.BackupType, rule.Metadata, rule.Match ? "Update" : "Add", () => overlay.close()),
			match = input({ "id": "match", "type": "text", "value": rule.Match, "disabled": !!rule.Match }),
			override = input({ "id": "override", "type": "checkbox", "checked": rule.Override, "disabled": !!rule.Match }),
			disableInputs = () => {
				override.toggleAttribute("disabled", true);
				overlay.setAttribute("closedby", "none");
				set.toggleAttribute("disabled", true);
				cancel.toggleAttribute("disabled", true);
				backupType.toggleAttribute("disabled", true);
			},
			enableInputs = () => {
				override.removeAttribute("disabled");
				overlay.setAttribute("closedby", "any");
				set.removeAttribute("disabled");
				cancel.removeAttribute("disabled");
				backupType.removeAttribute("disabled");
			},
			overlay = document.body.appendChild(dialog({ "id": "addEdit", "closedby": "any", "close": () => overlay.remove() }, form({
				"submit": (e: SubmitEvent) => {
					e.preventDefault();

					disableInputs();

					verifyMetadata(path, backupType.value, metadata.value)
						.then(valid => {
							if (!valid) {
								alert("Set does not exist");
								enableInputs();

								return;
							}

							return (rule.Match ? updateRule : createRule)(path, backupType.value, [rule.Match || match.value || "*"], BackupType.from(backupType.value).isManual() ? metadata.value : "", override.checked)
								.then(() => {
									load(path);
									overlay.remove();
									updateClaimStats();
								});
						})
						.catch((e: Error) => {
							enableInputs();
							alert("Error: " + e.message);
						});
				}
			}, [
				label({ "for": "match" }, "Match"), getHelpIcon(helpText.Match), match, br(),
				label({ "for": "override" }, "Override Child Rules"), getHelpIcon(helpText.Override), override, br(),
				label({ "for": "backupType" }, "Backup Type"), getHelpIcon(helpText.BackupType), backupType, br(),
				metadataSection,
				set,
				cancel
			])));

		overlay.showModal();
	},
	loadFileContents = (file: File) => new Promise<string>(resolve => {
		const fr = new FileReader();

		fr.onload = () => resolve(fr.result as string);

		fr.readAsText(file);
	}),
	parseFOFN = (base: string, contents: string) => Array.from(new Set(contents.split("\n").map(line => {
		line = new URL(line.replace(/#.*/, "").trim(), "a://a").pathname.slice(+!line.startsWith("/"));

		if (line.endsWith("/")) {
			line += "*";
		}

		if (line.startsWith(base)) {
			line = line.slice(base.length)
		}

		return line;
	}).filter(line => line))),
	setPathError = (m: Map<string, string[]>, err: string, path: string) => (m.get(err) ?? setAndReturn(m, err, [])).push(path),
	validateFOFN = (base: string, contents: string, existingRules: Set<string>) => {
		const valid: string[] = [],
			invalid = new Map<string, string[]>();

		for (const line of parseFOFN(base, contents)) {
			if (line.includes("\x00")) {
				setPathError(invalid, "Invalid char in match", line);
			} else if (line.startsWith("/")) {
				setPathError(invalid, "Outside of current dir", line);
			} else if (line.startsWith("../") || line.includes("/../") || line.endsWith("/..")) {
				setPathError(invalid, "Cannot use relative paths to reference parent directories", line);
			} else if (existingRules.has(line)) {
				setPathError(invalid, "Duplicate", line);
			} else {
				valid.push(line);
			}
		}

		return [valid, invalid] as const;
	},
	buildFOFNTable = (base: string, contents: string, existingRules: Set<string>, fofnSection: HTMLDivElement, validRules: string[]) => {
		const [valid, invalid] = validateFOFN(base, contents, existingRules);

		clearNode(fofnSection, [
			h2("Rules from FOFN"),
			valid.length ? [
				ul(valid.map(rule => li(rule)))
			] : h3("No valid rules found"),
			invalid.size ? [
				h2("Invalid Filepaths"),
				table([
					thead(tr([
						th("Error"),
						th("Rule(s)"),
					])),
					tbody(Array.from(invalid.entries()).map(([error, rules]) => tr([
						td(error),
						td(ul(rules.map(rule => li(rule))))
					])))
				])
			] : []
		]);

		validRules.splice(0, validRules.length, ...valid);
	},
	addFofnOverlay = (path: string, existingRules: Set<string>) => {
		const [backupType, set, cancel, metadata, metadataSection] = createStuff(BackupType.BackupIBackup, "", "Add", () => overlay.close()),
			override = input({ "id": "override", "type": "checkbox" }),
			validRules: string[] = [],
			fofn = input({
				"id": "fofn", "type": "file", "style": "display: none",
				"change": () => loadFileContents(fofn.files![0])
					.then(contents => buildFOFNTable(path, contents, existingRules, fofnSection, validRules))
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
										buildFOFNTable(path, contents.value, existingRules, fofnSection, validRules);
										pasteOverlay.close();
									}
								}, "Add"),
								button({ "type": "button", "click": () => pasteOverlay.close() }, "Cancel")
							]
						));

					pasteOverlay.showModal();
				}
			}, "Paste as plain text"),
			fofnSection = div({ "id": "fofnDetails" }),
			disableInputs = () => {
				overlay.setAttribute("closedby", "none");
				override.toggleAttribute("disabled", true);
				set.toggleAttribute("disabled", true);
				cancel.toggleAttribute("disabled", true);
				backupType.toggleAttribute("disabled", true);
				fofnButton.toggleAttribute("disabled", true);
				fofnPaste.toggleAttribute("disabled", true);
				fofn.toggleAttribute("disabled", true);
			},
			enableInputs = () => {
				overlay.setAttribute("closedby", "any");
				override.removeAttribute("disabled");
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

					verifyMetadata(path, backupType.value, metadata.value)
						.then(valid => {
							if (!valid) {
								alert("Set does not exist");
								enableInputs();

								return;
							}

							return (validRules.length ? createRule(path, backupType.value, validRules, metadata.value, override.checked) : Promise.reject({ "message": "No FOFN selected" }))
								.then(() => {
									load(path);
									overlay.remove();
								})
								.catch(e => {
									alert(e.message);
									enableInputs();
								});
						})
				}
			}, [
				div({ "id": "matchfofn" }, [
					label({ "for": "fofn" }, "Add FOFN"),
					getHelpIcon(helpText.FOFN),
					fofnButton,
					fofn,
					" or ",
					fofnPaste
				]),
				label({ "for": "override" }, "Override Child Rules"), getHelpIcon(helpText.Override), override, br(),
				label({ "for": "backupType" }, "Backup Type"), getHelpIcon(helpText.BackupType), backupType, br(),
				metadataSection,
				fofnSection,
				set,
				cancel
			])));

		overlay.showModal();
	},
	dirDetailOverlay = (path: string, dirDetails: dirDetails) => {
		const frequency = input({ "id": "frequency", "type": "number", "min": "0", "value": dirDetails.Frequency + "" }),
			frozen = input({ "id": "frozen", "type": "checkbox", "checked": dirDetails.Frozen }),
			review = input({ "id": "review", "type": "date", "value": new Date(dirDetails.ReviewDate * 1000).toISOString().substring(0, 10) }),
			remove = input({ "id": "remove", "type": "date", "value": new Date(dirDetails.RemoveDate * 1000).toISOString().substring(0, 10) }),
			set = button({ "value": "set" }, "Set"),
			cancel = button({ "type": "button", "click": () => overlay.close() }, "Cancel"),
			disableInputs = () => {
				overlay.setAttribute("closedby", "none");
				set.toggleAttribute("disabled", true);
				cancel.toggleAttribute("disabled", true);
				frequency.toggleAttribute("disabled", true);
				frozen.toggleAttribute("disabled", true);
				review.toggleAttribute("disabled", true);
				remove.toggleAttribute("disabled", true);
			},
			enableInputs = () => {
				overlay.setAttribute("closedby", "any");
				set.removeAttribute("disabled");
				cancel.removeAttribute("disabled");
				frequency.removeAttribute("disabled");
				frozen.removeAttribute("disabled");
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

					setDirDetails(path, frequency.valueAsNumber, frozen.checked, + review.valueAsDate / 1000, + remove.valueAsDate / 1000)
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
				label({ "for": "frequency" }, "Frequency"), getHelpIcon(helpText.Frequency), frequency, br(),
				label({ "for": "frozen" }, "Frozen"), getHelpIcon(helpText.Frozen), frozen, br(),
				label({ "for": "review" }, "Review Date"), getHelpIcon(helpText.Review), review, br(),
				label({ "for": "remove" }, "Remove Date"), getHelpIcon(helpText.Remove), remove, br(),
				set,
				cancel
			])));

		overlay.showModal();
	},
	addRule = (path: string) => button({
		"click": () => addEditOverlay(path, {
			"BackupType": BackupType.BackupIBackup,
			"Metadata": "",
			"Match": "",
			"Override": false
		}),
	}, "Add Rule"),
	addFOFN = (path: string, existingRules: RuleStats[]) => button({
		"click": () => addFofnOverlay(path, new Set(existingRules.map(r => r.Match))),
	}, "Add FOFN"),
	addDirDetails = (path: string, dirDetails: dirDetails) => button({
		"click": () => dirDetailOverlay(path, dirDetails),
	}, "Set Directory Details"),
	base = div();

export default base;

registerLoader((path: string, data: DirectoryWithChildren) => {
	clearNode(base, [
		data.claimedBy ? h2("Rules on this directory") : [],
		data.claimedBy && data.claimedBy == user && !data.rules[path]?.length ? [addRule(path), addFOFN(path, data.rules[path]), addDirDetails(path, data)] : [],
		data.claimedBy && data.rules[path]?.length ? table({ "id": "rules", "class": "summary" }, [
			thead(tr([th("Match"), th("Action"), th("Files"), th("Size"), data.claimedBy === user ? td([addRule(path), addFOFN(path, data.rules[path]), addDirDetails(path, data)]) : []])),
			tbody(Object.values(data.rules[path] ?? []).map(rule => tr([
				td({ "data-override": rule.Override }, rule.Match),
				td(action(rule.BackupType)),
				td(rule.count.toLocaleString()),
				td({ "title": rule.size.toLocaleString() }, formatBytes(rule.size)),
				data.claimedBy === user ? td([
					button({
						"class": "actionButton",
						"click": () => addEditOverlay(path, rule)
					}, svg([
						title("Edit Rule"),
						use({ "href": "#edit" })
					])),
					button({
						"class": "actionButton",
						"click": () => confirm("Are you sure you wish to remove this rule?", () => removeRule(path, rule.Match).then(() => { load(path); updateClaimStats(); }))
					}, svg([
						title("Remove Rule"),
						use({ "href": "#remove" })
					]))
				]) : []
			])))
		]) : []
	]);
});