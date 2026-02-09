import type { BackupStatus, ClaimedDir, ReportSummary, Rule, SizeCount, SizeCountTime, Stats } from "./types.js";
import type { Children } from "./lib/dom.js";
import { amendNode } from "./lib/dom.js";
import { a, br, button, datalist, details, div, fieldset, h1, h2, input, label, legend, li, option, p, span, summary, table, tbody, td, th, thead, tr, ul } from "./lib/html.js";
import { svg, title, use } from "./lib/svg.js";
import { action, formatBytes, longAgo, secondsInWeek, setAndReturn, stringSort } from "./lib/utils.js";
import { getReportSummary } from "./rpc.js";
import { BackupType, MainProgrammes } from "./consts.js";
import { render } from "./disktree.js";
import { load } from './load.js';
import ODS from './odf.js';
import { boms, owners, userGroups } from './userGroups.js';
import { inputState } from "./state.js";
import graph from "./graph.js";

class Summary {
	actions: SizeCountTime[] = [];
	path: string;
	lastestMTime = 0;
	count = 0n;
	backupStatus?: BackupStatus;
	backups = new Map<string, BackupStatus[]>();

	constructor(path: string, backupStatus?: BackupStatus) {
		this.path = path;
		this.backupStatus = backupStatus;
	}

	add(action: BackupType, rule: Stats) {
		const sct = this.actions[+action] ??= { size: 0n, count: 0n, mtime: 0 };

		sct.count += BigInt(rule.Files);
		sct.size += BigInt(rule.Size);
		sct.mtime = Math.max(sct.mtime, rule.MTime);

		this.lastestMTime = Math.max(this.lastestMTime, rule.MTime);
		this.count += BigInt(rule.Files);
	}

	addBackupStatus(path: string, backupStatus: BackupStatus) {
		(this.backups.get(path) ?? setAndReturn(this.backups, path, [])).push(backupStatus);
	}

	table() {
		const manualSizeCount = this.manualSizeCount();

		return table({ "class": "summary" }, [
			thead(tr([
				td(),
				th("Unplanned"),
				th("No Backup"),
				th("Backup"),
				th("Manual Backup")
			])),
			tbody([
				tr([
					th("File count"),
					td(this.actions[+BackupType.BackupWarn]?.count.toLocaleString() ?? "0"),
					td(this.actions[+BackupType.BackupNone]?.count.toLocaleString() ?? "0"),
					td(this.actions[+BackupType.BackupIBackup]?.count.toLocaleString() ?? "0"),
					td(manualSizeCount.count.toLocaleString())
				]),
				tr([
					th("File size"),
					td({ "title": (this.actions[+BackupType.BackupWarn]?.size ?? 0).toLocaleString() }, formatBytes(this.actions[+BackupType.BackupWarn]?.size ?? 0)),
					td({ "title": (this.actions[+BackupType.BackupNone]?.size ?? 0).toLocaleString() }, formatBytes(this.actions[+BackupType.BackupNone]?.size ?? 0)),
					td({ "title": (this.actions[+BackupType.BackupIBackup]?.size ?? 0).toLocaleString() }, formatBytes(this.actions[+BackupType.BackupIBackup]?.size ?? 0)),
					td({ "title": manualSizeCount.size.toLocaleString() }, formatBytes(manualSizeCount.size))
				])
			])
		])
	}

	manualSizeCount() {
		return this.actions.filter((_, n) => BackupType.from(n).isManual()).reduce((prev, next) => (prev.size += next.size, prev.count += next.count, prev), { size: 0n, count: 0n });
	}
}

class ParentSummary extends Summary {
	children = new Map<string, ChildSummary>();
	group: string;

	constructor(path: string, group: string, backupStatus?: BackupStatus) {
		super(path, backupStatus);

		this.group = group;
	}

	addChild(child: string, rule: Rule, stats: Stats) {
		const c = this.children.get(child) ?? setAndReturn(this.children, child, new ChildSummary(child));

		if (rule) {
			c.addRule(rule.Match, rule.BackupType, stats);
		}
	}

	section() {
		return fieldset({
			"data-status": this.status(),
			"data-warn-size": (this.actions[+BackupType.BackupWarn]?.size ?? 0) + "",
			"data-nobackup-size": (this.actions[+BackupType.BackupNone]?.size ?? 0) + "",
			"data-backup-size": (this.actions[+BackupType.BackupIBackup]?.size ?? 0) + "",
			"data-name": this.path,
			"data-group": this.group,
			"data-bom": boms.get(this.group)!,
			"data-owner": owners.get(this.group)!,
		}, [
			legend(h1([
				this.path,
				button({
					"click": () => load(this.path).then(() => {
						window.scrollTo(0, 0);
						document.getElementsByTagName("summary")[0].click();
						render();
					})
				}, svg([
					title("Go to"),
					use({ "href": "#goto" })
				]))
			])),
			ul([
				this.backupStatus ? li("Requester: " + this.backupStatus.Requester) : [],
				this.actions[+BackupType.BackupIBackup]?.mtime ? li("Last Activity in Backed-up Set: " + longAgo(this.actions[+BackupType.BackupIBackup]?.mtime ?? 0)) : [],
				li("Last Activity: " + (this.lastestMTime ? longAgo(this.lastestMTime) : "--none--"))
			]),
			this.table(),
			table({ "class": "summary" }, [
				thead(tr([
					th("Directory"),
					th("Claimed By"),
					th("Backup Name"),
					th("Last Backup"),
					th("Failures")
				])),
				tbody(this.backups.size ? [
					Array.from(this.backups).map(([path, backups]) => backups.map(backup => tr([
						td(path),
						td(backup.Requester),
						td(backup.Name),
						td(
							backup.LastSuccess
								// If status exists but is equal to zero time (ibackup broken) show pending
								? +new Date(backup.LastSuccess) <= 0
									? "Pending"
									: new Date(backup.LastSuccess).toLocaleString()
								: "-"
						),
						td(backup.Failures.toLocaleString())
					])))
				] : tr(td({ "colspan": "5" }, "No Backups")))
			]),
			this.children.size ? [
				h2("Rules"),
				Array.from(this.children.entries()).map(([path, child]) => details([
					summary(path),
					child.section()
				]))
			] : []
		])
	}

	status() {
		const now = (+new Date()) / 1000,
			backupTime = this.actions[+BackupType.BackupIBackup]?.mtime ?? 0,
			lastActivity = Math.max(now - this.lastestMTime, 0),
			lastBackupActivity = Math.max(now - backupTime),
			dt = lastBackupActivity - lastActivity;

		return ((this.actions[+BackupType.BackupNone]?.count ?? 0n) !== this.count) ? dt < secondsInWeek ? "g" : dt < secondsInWeek * 3 ? "a" : "r" : "b"
	}
}

class ChildSummary extends Summary {
	rules = new Map<string, SizeCount & { action: BackupType }>();

	constructor(path: string) {
		super(path);
	}

	addRule(match: string, action: BackupType, rule: Stats) {
		const r = this.rules.get(match) ?? setAndReturn(this.rules, match, { size: 0n, count: 0n, action });

		r.count += BigInt(rule.Files);
		r.size += BigInt(rule.Size);

		this.add(action, rule);
	}

	section() {
		const tables: HTMLElement[] = [];

		tables.push(this.table());

		const validRules = Array.from(this.rules.entries())
			.filter(([_, rule]) => rule && rule.action !== BackupType.BackupWarn);

		// Only show extra table if there are rules to show
		if (validRules.length === 0) {
			return tables;
		}

		const ruleTable = table({ "class": "summary" }, [
			thead(tr([th("Match"), th("Action"), th("Files"), th("Size")])),
			tbody(validRules.map(([match, rule]) => tr([
				td(match),
				td(action(rule.action)),
				td(rule.count.toLocaleString()),
				td({ "title": rule.size.toLocaleString() }, formatBytes(rule.size))
			])))
		])

		tables.push(ruleTable);

		return tables;
	}
}

const groupList = datalist({ "id": "groupList" }),
	base = div({ "id": "report" }, groupList),
	initFilterSort = (container: HTMLDivElement, children: HTMLFieldSetElement[], [filterProject, filterAll, filterR, filterA, filterG, filterB, sortName, sortWarnSize, sortNoBackupSize, sortBackupSize]: [HTMLInputElement, HTMLInputElement, HTMLInputElement, HTMLInputElement, HTMLInputElement, HTMLInputElement, HTMLInputElement, HTMLInputElement, HTMLInputElement, HTMLInputElement]) => {
		const projects = children.map(child => ({
			"elem": child,
			"name": child.dataset.name!,
			"warnSize": parseInt(child.dataset.warnSize!),
			"nobackupSize": parseInt(child.dataset.nobackupSize!),
			"backupSize": parseInt(child.dataset.backupSize!),
			"status": child.dataset.status!,
			"owner": child.dataset.owner || null,
			"bom": child.dataset.bom || null,
			"group": child.dataset.group || null,
		})),
			stringSort = new Intl.Collator().compare,
			filterData = { "name": "", "status": "" },
			filter = (key: "name" | "status", value: string) => {
				filterData[key] = value;
				for (const project of projects) {
					let hideProject = false;
					let skipName = false;
					if (!project.status.includes(filterData["status"])) {
						hideProject = true;
					}

					if (project.owner === filterData['name'] ||
						project.bom === filterData['name'] ||
						project.group === filterData['name']
					) { skipName = true; }

					if (!skipName && !project.name.toLowerCase().includes(filterData['name'].toLowerCase())) {
						hideProject = true;
					}

					project.elem.classList.toggle("h", hideProject);
				}
			},
			sort = (key: "name" | "warnSize" | "nobackupSize" | "backupSize") => {
				projects.sort((a, b) => key === "name" ? stringSort(a[key], b[key]) : b[key] - a[key]);
				container.append(...projects.map(p => p.elem));
			};

		filterProject.addEventListener("input", () => filter("name", filterProject.value));
		filterAll.addEventListener("click", () => filter("status", ""));
		filterR.addEventListener("click", () => filter("status", "r"));
		filterA.addEventListener("click", () => filter("status", "a"));
		filterG.addEventListener("click", () => filter("status", "g"));
		filterB.addEventListener("click", () => filter("status", "b"));

		sortName.addEventListener("click", () => sort("name"));
		sortWarnSize.addEventListener("click", () => sort("warnSize"));
		sortNoBackupSize.addEventListener("click", () => sort("nobackupSize"));
		sortBackupSize.addEventListener("click", () => sort("backupSize"));
	},
	download = () => {
		const getCSS = (sheet: CSSStyleSheet): string => Array.from(sheet.cssRules, rule => rule instanceof CSSImportRule ? getCSS(rule.styleSheet!) : rule.cssText).reduce((a, b) => a + b, ""),
			// Ignore first input box when filtering as its a hidden checkbox for expand/collapse table functionality
			html = `<!DOCTYPE html>
<html lang="en"><head><title>Backup Report - ${new Date(now).toLocaleString()}</title><style type="text/css">${Array.from(document.styleSheets, getCSS).reduce((a, b) => a + b, "")}</style><script type="module">(document.readyState === "complete" ? Promise.resolve() : new Promise(successFn => window.addEventListener("load", successFn, { "once": true }))).then(() => (${initFilterSort.toString().replace(/\n\t/g, "")})(document.getElementById("report"), Array.from(document.getElementsByTagName("fieldset")).filter(f => f.dataset.name), Array.from(document.getElementsByTagName("input")).slice(1)));</script></head><body>${base.outerHTML}</body></html>`;

		a({ "href": URL.createObjectURL(new Blob([html], { "type": "text/html;charset=utf-8" })), "download": `backup-report-${new Date(now).toISOString()}.html` }).click();
	};

let now = 0,
	summaryData: ReportSummary;

function renderCell(counts: Map<BackupType, SizeCount>, type: BackupType) {
	const cells = [
		td(counts.get(type)?.count.toLocaleString() ?? "0"),
		td({ "title": counts.get(type)?.size.toLocaleString() ?? "0" }, formatBytes(BigInt(counts.get(type)?.size ?? 0n))),
	]

	if (type === BackupType.BackupWarn) {
		cells.push(td(getUnplannedFraction(counts).toLocaleString()));
	}

	return cells
};

function renderTotalCell(counts: Map<BackupType, SizeCount>) {
	const totals = Array.from(counts.values()).reduce((sc, acc) => (
		{ count: acc.count += sc.count, size: acc.size += sc.size }), { count: 0n, size: 0n });

	return [
		td(totals.count.toLocaleString() ?? "0"),
		td({ "title": totals.size.toLocaleString() ?? "0" }, formatBytes(totals.size ?? 0n)),
	]
}

function renderRow(counts: Map<BackupType, SizeCount>) {
	return [
		[BackupType.BackupWarn, BackupType.BackupNone, BackupType.BackupIBackup, BackupType.BackupManual].flatMap(t => renderCell(counts, t)),
		renderTotalCell(counts)
	];
}

function getUnplannedFraction(counts: Map<BackupType, SizeCount>) {
	let totalSize = 0n;

	for (const sizeCount of counts.values()) {
		totalSize += sizeCount.size
	}

	const unplannedSize = counts.get(BackupType.BackupWarn)?.size ?? 0n;

	return Number(unplannedSize * 100n / totalSize) / 100
}

getReportSummary()
	.then(data => {
		userGroups
		summaryData = data;
		now = +new Date();

		const children: Children = ["", "", ""],
			parents: ParentSummary[] = [],
			overall = new Summary(""),
			filterProject = inputState({ "id": "reportFilter", "placeholder": "Name, BOM, Group, Owner", "list": "groupList" }),
			filterAll = input({ "id": "filterAll", "name": "filter", "type": "radio", "checked": "checked" }),
			filterR = input({ "id": "filterRed", "name": "filter", "type": "radio" }),
			filterA = input({ "id": "filterAmber", "name": "filter", "type": "radio" }),
			filterG = input({ "id": "filterGreen", "name": "filter", "type": "radio" }),
			filterB = input({ "id": "filterBlue", "name": "filter", "type": "radio" }),
			sortName = input({ "id": "sortName", "name": "sort", "type": "radio", "checked": "checked" }),
			sortWarnSize = input({ "id": "sortWarnSize", "name": "sort", "type": "radio" }),
			sortNoBackupSize = input({ "id": "sortNoBackupSize", "name": "sort", "type": "radio" }),
			sortBackupSize = input({ "id": "sortBackupSize", "name": "sort", "type": "radio" });

		for (const rule of Object.values(data.Rules)) {
			rule.BackupType = BackupType.from(rule.BackupType);
		}

		for (const [dir, summary] of Object.entries(data.Summaries)) {
			const dirSummary = new ParentSummary(dir, summary.Group, data.BackupStatus[dir]);

			for (const ruleSummary of summary.RuleSummaries) {
				const rule = data.Rules[ruleSummary.ID],
					ruleType = rule?.BackupType ?? BackupType.BackupWarn;

				for (const user of ruleSummary.Users) {
					overall.add(ruleType, user);
					dirSummary.add(ruleType, user);
				}
			}

			for (const [childDir, childSummary] of [[dir, summary] as [string, ClaimedDir]].concat(Array.from(Object.entries(summary.Children)))) {
				const rules = data.Directories[childDir] ?? [0];

				for (const id of rules) {
					const rule = data.Rules[id] ?? { BackupType: BackupType.BackupWarn, Match: "" };

					switch (rule.BackupType) {
						case BackupType.BackupIBackup:
							dirSummary.addBackupStatus(childDir, data.BackupStatus[childDir]);

							break;
						case BackupType.BackupManualIBackup:
							dirSummary.addBackupStatus(childDir, data.BackupStatus[childSummary.ClaimedBy + ":" + rule.Metadata]);

							break;
						case BackupType.BackupManualGit:
							dirSummary.addBackupStatus(childDir, data.BackupStatus[rule.Metadata]);
					}

					for (const stats of summary.RuleSummaries.find(v => v.ID === id)?.Users ?? []) {
						dirSummary.addChild(childDir, rule, stats);
					}
				}
			}

			children.push(dirSummary.section());
			parents.push(dirSummary);
		}

		// const programmeCounts = new Map<string, Map<BackupType, SizeCount>>(); // Programme -> BackupType -> SizeCount

		// // const programmeCounts = new Map<string, Map<number, SizeCount>>(); // Programme -> BackupType -> SizeCount
		// programmeCounts.set("All", new Map<BackupType, SizeCount>())

		// for (const [group, typeCounts] of Object.entries(data.GroupBackupTypeTotals)) {
		// 	const bom = (!boms.get(group) || boms.get(group) === "unknown") ? "Unknown" : boms.get(group)!;

		// 	for (const [type, sizeCounts] of Object.entries(typeCounts)) {
		// 		const backupType = BackupType.from(type),
		// 			mcBackupType = backupType.isManual() ? BackupType.BackupManual : backupType,
		// 			counts = programmeCounts.get(bom) ?? setAndReturn(programmeCounts, bom, new Map<BackupType, SizeCount>());

		// 		const programmeSizeCount = counts.get(mcBackupType) ?? setAndReturn(counts, mcBackupType, { count: 0n, size: 0n });

		// 		programmeSizeCount.size += BigInt(sizeCounts.size);
		// 		programmeSizeCount.count += BigInt(sizeCounts.count);

		// 		setCountsAll(programmeCounts, mcBackupType, sizeCounts)
		// 	}
		// }
		const programmeCounts = buildProgrammeCounts(data.GroupBackupTypeTotals);
		graph.init(programmeCounts);

		// const rows = Array.from(programmeCounts.entries())
		// 	.filter(e => e[0])
		// 	.sort(sortProgrammes)
		// 	.map(([bom, counts]) => {
		// 		return [
		// 			tr([
		// 				th(bom),
		// 				...[BackupType.BackupWarn, BackupType.BackupNone, BackupType.BackupIBackup, BackupType.BackupManual].flatMap(t => renderCell(counts, t))
		// 			])
		// 		];
		// 	});

		// const rows = Array.from(programmeCounts.entries())
		// 	.filter(e => e[0])
		// 	.sort(sortProgrammes)
		// 	.map(([bom, counts]) => tr([
		// 		th(bom),
		// 		renderRow(counts)
		// 	]));

		children[0] = div({ "class": "summary-container" }, [
			input({ "type": "checkbox", "id": "tableToggleCheckbox", "style": "display:none" }),
			table({ "class": "summary" }, [
				thead(tr([
					td([
						button({ "click": download }, "Download Report"),
						button({
							"click": () => {
								const ods = ODS(
									parents.map(s => ({
										"Programme": boms.get(s.group) ?? "",
										"Faculty": owners.get(s.group) ?? "",
										"Path": s.path,
										"Group": s.group,
										"Status": s.status(),
										"Unplanned": s.actions[+BackupType.BackupWarn]?.size ?? 0n,
										"NoBackup": s.actions[+BackupType.BackupNone]?.size ?? 0n,
										"Backup": s.actions[+BackupType.BackupIBackup]?.size ?? 0n,
										"ManualBackup": getManualSize(s)
									})),
									Array.from(programmeCounts.entries()).map(([bom, counts]) => ({
										"Programme": bom,
										"UnplannedC": BigInt(counts.get(BackupType.BackupWarn)?.count ?? 0n),
										"UnplannedS": BigInt(counts.get(BackupType.BackupWarn)?.size ?? 0n),
										"UnplannedF": Math.round(1000 * (Number(counts.get(BackupType.BackupWarn)?.size ?? 0n)) /
											(Number(counts.get(BackupType.BackupWarn)?.size ?? 0n) + Number(counts.get(BackupType.BackupNone)?.size ?? 0n) + Number(counts.get(BackupType.BackupIBackup)?.size ?? 0n) + Number(counts.get(BackupType.BackupManual)?.size ?? 0n))) / 1000,
										"NoBackupC": BigInt(counts.get(BackupType.BackupNone)?.count ?? 0n),
										"NoBackupS": BigInt(counts.get(BackupType.BackupNone)?.size ?? 0n),
										"BackupC": BigInt(counts.get(BackupType.BackupIBackup)?.count ?? 0n),
										"BackupS": BigInt(counts.get(BackupType.BackupIBackup)?.size ?? 0n),
										"ManualC": BigInt(counts.get(BackupType.BackupManual)?.count ?? 0n),
										"ManualS": BigInt(counts.get(BackupType.BackupManual)?.size ?? 0n)
									})));
								a({ "href": URL.createObjectURL(new Blob([ods], { "type": "text/csv;charset=utf-8" })), "download": `backup-report-${new Date(now).toISOString()}.ods` }).click();
							}
						}, "Download Spreadsheet")
					]),
					th({ "colspan": "3" }, "Unplanned (Count/Size/Fraction)"),
					th({ "colspan": "2" }, "No Backup (Count/Size)"),
					th({ "colspan": "2" }, "Backup (Count/Size)"),
					th({ "colspan": "2" }, "Manual Backup (Count/Size)"),
					th({ "colspan": "2" }, "Overall (Count/Size)")
				])),
				tbody([
					Array.from(programmeCounts.entries())
						.sort(sortProgrammes)
						.map(([bom, counts]) => tr([
							th(bom),
							renderRow(counts)
						])),
					tr({ "id": "tableCollapse" }, td({ "colspan": "12" }, label({ "for": "tableToggleCheckbox" }, [span({ "class": "expand-text" }, "Expand All"), span({ "class": "collapse-text" }, "Collapse All")])))
				])
			])
		]);

		children[1] = fieldset([
			legend("Filter"),
			filterProject,
			br(),
			label({ "for": "filterAll" }, "All"), filterAll,
			label({ "for": "filterRed" }, "No backup in 6 weeks"), filterR,
			label({ "for": "filterAmber" }, "No backup in 2 weeks"), filterA,
			label({ "for": "filterGreen" }, "Backup within 2 weeks"), filterG,
			label({ "for": "filterBlue" }, "No files to backup"), filterB
		]);
		children[2] = fieldset([
			legend("Sort"),
			label({ "for": "sortName" }, "Name"), sortName,
			label({ "for": "sortWarnSize" }, "Unplanned size"), sortWarnSize,
			label({ "for": "sortNoBackupSize" }, "No Backup Size"), sortNoBackupSize,
			label({ "for": "sortBackupSize" }, "Backup Size"), sortBackupSize
		]);

		initFilterSort(base, children.slice(3) as HTMLFieldSetElement[], [filterProject, filterAll, filterR, filterA, filterG, filterB, sortName, sortWarnSize, sortNoBackupSize, sortBackupSize]);
		amendNode(base, children);
	});

function getManualSize(s: ParentSummary) {
	return BackupType.manual.reduce((total, backup) => total + (s.actions[+backup]?.size ?? 0n), 0n);
}

function sortProgrammes([progA, countsA]: [string, Map<BackupType, SizeCount>], [progB, countsB]: [string, Map<BackupType, SizeCount>]) {
	if (progA === "All") {
		return -1;
	}

	if (progB === "All") {
		return 1;
	}

	const fractionA = getUnplannedFraction(countsA),
		fractionB = getUnplannedFraction(countsB),
		isPriorityA = MainProgrammes.includes(progA),
		isPriorityB = MainProgrammes.includes(progB);

	if (isPriorityA && isPriorityB) {
		return Number(fractionB - fractionA);
	}

	if (isPriorityA) {
		return -1;
	}

	if (isPriorityB) {
		return 1;
	}

	return Number(fractionB - fractionA);
}


function setCountsAll(programmeCounts: Map<string, Map<BackupType, SizeCount>>, backupType: BackupType, sizeCounts: SizeCount, bom: string) {
	const all = programmeCounts.get("All") ?? setAndReturn(programmeCounts, "All", new Map<BackupType, SizeCount>()),
		allTotals = all.get(backupType) ?? setAndReturn(all, backupType, { count: 0n, size: 0n });
	const nqAll = programmeCounts.get("")!;

	allTotals.size += BigInt(sizeCounts.size);
	allTotals.count += BigInt(sizeCounts.count);

	if (!MainProgrammes.includes(bom)) {
		return
	}

	if (!nqAll.has(backupType)) {
		nqAll.set(backupType, { count: 0n, size: 0n });
	}

	const nqAllTotals = nqAll.get(backupType)!;
	nqAllTotals.size += BigInt(sizeCounts.size);
	nqAllTotals.count += BigInt(sizeCounts.count);
}


for (const [bom, groups] of Object.entries(userGroups.BOM ?? {})) {
	for (const group of groups) {
		boms.set(group, bom);
	}
}

for (const [owner, groups] of Object.entries(userGroups.Owners ?? {})) {
	for (const group of groups) {
		owners.set(group, owner);
	}
}

const groups = userGroups.Groups.filter(g => g.trim()),
	gArr = [];

groups.sort(stringSort);

for (const [bom, groups] of Object.entries(userGroups.BOM ?? {})) {
	gArr.push(["BOM: ", bom]);
}

for (const [owner] of Object.entries(userGroups.Owners ?? {})) {
	gArr.push(["Owner :", owner]);
}

groups.forEach(g => gArr.push(["Group :", g]));
gArr.sort((a, b) => a[1].localeCompare(b[1]));
gArr.forEach(([k, v]) => groupList.append(option({ "label": k + v }, v)));

export default base;

function buildProgrammeCounts(GroupBackupTypeTotals: Record<string, Record<number, SizeCount>>) {
	const programmeCounts = new Map<string, Map<BackupType, SizeCount>>(); // Programme -> BackupType -> SizeCount
	programmeCounts.set("All", new Map<BackupType, SizeCount>());
	programmeCounts.set("", new Map<BackupType, SizeCount>());

	for (const [group, typeCounts] of Object.entries(GroupBackupTypeTotals)) {
		const bom = (!boms.get(group) || boms.get(group) === "unknown") ? "Unknown" : boms.get(group)!;

		for (const [type, sizeCounts] of Object.entries(typeCounts)) {
			const backupType = BackupType.from(type),
				mcBackupType = backupType.isManual() ? BackupType.BackupManual : backupType,
				counts = programmeCounts.get(bom) ?? setAndReturn(programmeCounts, bom, new Map<BackupType, SizeCount>());

			const programmeSizeCount = counts.get(mcBackupType) ?? setAndReturn(counts, mcBackupType, { count: 0n, size: 0n });

			programmeSizeCount.size += BigInt(sizeCounts.size);
			programmeSizeCount.count += BigInt(sizeCounts.count);

			setCountsAll(programmeCounts, mcBackupType, sizeCounts, bom)
		}
	}
	// for (const [group, typeCounts] of Object.entries(GroupBackupTypeTotals)) {
	// 	const bom = (!boms.get(group) || boms.get(group) === "unknown") ? "Unknown" : boms.get(group)!;

	// 	if (!programmeCounts.has(bom)) {
	// 		programmeCounts.set(bom, new Map<BackupType, SizeCount>());
	// 	}

	// 	for (const [type, sizeCounts] of Object.entries(typeCounts)) {
	// 		const backupType = BackupType.from(type);

	// 		const counts = programmeCounts.get(bom)!;

	// 		if (!counts.has(backupType)) {
	// 			counts.set(backupType, { count: 0n, size: 0n });
	// 		}

	// 		const sizecount = counts.get(backupType)!;
	// 		sizecount.size += BigInt(sizeCounts.size);
	// 		sizecount.count += BigInt(sizeCounts.count);

	// 		setCountsAll(programmeCounts, backupType, sizeCounts, bom)
	// 	}
	// }
	console.log("programmeCounts", programmeCounts);
	return programmeCounts
}
