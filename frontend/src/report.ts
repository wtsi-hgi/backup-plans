import type { BackupStatus, BackupType, ClaimedDir, DirectoryWithChildren, Rule, SizeCount, SizeCountTime, Stats } from "./types.js";
import type { Children } from "./lib/dom.js";
import { amendNode } from "./lib/dom.js";
import { a, br, button, details, div, fieldset, h1, h2, input, label, legend, li, summary, table, tbody, td, th, thead, tr, ul } from "./lib/html.js";
import { svg, title, use } from "./lib/svg.js";
import { action, formatBytes, longAgo, secondsInWeek, setAndReturn } from "./lib/utils.js";
import { getReportSummary } from "./rpc.js";
import { BackupIBackup, BackupNone } from "./types.js";
import { render } from "./disktree.js";


class Summary {
	actions: SizeCountTime[] = [];
	path: string;
	lastestMTime = 0;
	count = 0n;
	backupStatus?: BackupStatus;

	constructor(path: string, backupStatus?: BackupStatus) {
		this.path = path;
		this.backupStatus = backupStatus;
	}

	add(action: BackupType, rule: Stats) {
		const sct = this.actions[action] ??= { size: 0n, count: 0n, mtime: 0 };

		sct.count += BigInt(rule.Files);
		sct.size += BigInt(rule.Size);
		sct.mtime = Math.max(sct.mtime, rule.MTime);

		this.lastestMTime = Math.max(this.lastestMTime, rule.MTime);
		this.count += BigInt(rule.Files);
	}

	table() {
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
					td(this.actions[-1]?.count.toLocaleString() ?? "0"),
					td(this.actions[0]?.count.toLocaleString() ?? "0"),
					td(this.actions[1]?.count.toLocaleString() ?? "0"),
					td(this.actions[2]?.count.toLocaleString() ?? "0")
				]),
				tr([
					th("File size"),
					td({ "title": (this.actions[-1]?.size ?? 0).toLocaleString() }, formatBytes(this.actions[-1]?.size ?? 0)),
					td({ "title": (this.actions[0]?.size ?? 0).toLocaleString() }, formatBytes(this.actions[0]?.size ?? 0)),
					td({ "title": (this.actions[1]?.size ?? 0).toLocaleString() }, formatBytes(this.actions[1]?.size ?? 0)),
					td({ "title": (this.actions[2]?.size ?? 0).toLocaleString() }, formatBytes(this.actions[2]?.size ?? 0))
				])
			])
		])
	}
}

class ParentSummary extends Summary {
	children = new Map<string, ChildSummary>();

	addChild(child: string, rule: Rule, stats: Stats, claimedBy: string, backupStatus: BackupStatus) {
		const c = this.children.get(child) ?? setAndReturn(this.children, child, new ChildSummary(child, claimedBy, backupStatus));

		if (rule) {
			c.addRule(rule.Match, rule.BackupType, stats);
		}
	}

	section() {
		const now = (+new Date()) / 1000,
			backupTime = this.actions[BackupIBackup]?.mtime ?? 0,
			lastActivity = Math.max(now - this.lastestMTime, 0),
			lastBackupActivity = Math.max(now - backupTime),
			dt = lastBackupActivity - lastActivity;

		return fieldset({
			"data-status": ((this.actions[BackupNone]?.count ?? 0n) !== this.count) ? dt < secondsInWeek ? "g" : dt < secondsInWeek * 3 ? "a" : "r" : "b",
			"data-warn-size": (this.actions[-1]?.size ?? 0) + "",
			"data-nobackup-size": (this.actions[0]?.size ?? 0) + "",
			"data-backup-size": (this.actions[2]?.size ?? 0) + "",
			"data-name": this.path
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
				this.actions[BackupIBackup]?.mtime ? li("Last Activity in Backed-up Set: " + longAgo(this.actions[BackupIBackup]?.mtime ?? 0)) : [],
				li("Last Activity: " + (this.lastestMTime ? longAgo(this.lastestMTime) : "--none--"))
			]),
			this.table(),
			table({ "class": "summary" }, [
				thead(tr([
					th("Claimed By"),
					th("Set Name"),
					th("Last Backup"),
					th("Failures")
				])),
				tbody(this.children.size ? Array.from(this.children.entries()).map(([path, child]) => tr([
					td(child.claimedBy),
					td("plan::" + path),
					td(child.backupStatus ? new Date(child.backupStatus.LastSuccess).toLocaleString() : "-"),
					td(child.backupStatus?.Failures.toLocaleString() ?? "-")
				])) : tr(td({ "colspan": "4" }, "No Backups")))
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
}

class ChildSummary extends Summary {
	rules = new Map<string, SizeCount & { action: BackupType }>();
	claimedBy: string;

	constructor(path: string, claimedBy: string, backupStatus?: BackupStatus) {
		super(path, backupStatus);

		this.claimedBy = claimedBy;
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
				.filter(([_, rule]) => rule && rule.action !== -1);
		
		// Only show extra table if there are rules to show
		if (validRules.length === 0) {
			return tables;
		}

		const ruleTable = table({ "class": "summary" }, [
			thead(tr([th("Match"), th("Action"), th("Files"), th("Size")])),
			tbody(validRules.map(([match, rule]) => rule ? tr([
				td(match),
				td(action(rule.action)),
				td(rule.count.toLocaleString()),
				td({ "title": rule.size.toLocaleString() }, formatBytes(rule.size))
			]) : []))
		])

		tables.push(ruleTable);

		return tables;

	}
}

const base = div({ "id": "report" }),
	initFilterSort = (container: HTMLDivElement, children: HTMLFieldSetElement[], [filterProject, filterAll, filterR, filterA, filterG, filterB, sortName, sortWarnSize, sortNoBackupSize, sortBackupSize]: [HTMLInputElement, HTMLInputElement, HTMLInputElement, HTMLInputElement, HTMLInputElement, HTMLInputElement, HTMLInputElement, HTMLInputElement, HTMLInputElement, HTMLInputElement]) => {
		const projects = children.map(child => ({
			"elem": child,
			"name": child.dataset.name!,
			"warnSize": parseInt(child.dataset.warnSize!),
			"nobackupSize": parseInt(child.dataset.nobackupSize!),
			"backupSize": parseInt(child.dataset.backupSize!),
			"status": child.dataset.status!,
		})),
			stringSort = new Intl.Collator().compare,
			filterData = { "name": "", "status": "" },
			filter = (key: "name" | "status", value: string) => {
				filterData[key] = value.toLowerCase();
				for (const project of projects) {
					project.elem.classList.toggle("h", !project.name.includes(filterData["name"]) || !project.status.includes(filterData["status"]));
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
			html = `<!DOCTYPE html>
<html lang="en"><head><title>Backup Report - ${new Date(now).toLocaleString()}</title><style type="text/css">${Array.from(document.styleSheets, getCSS).reduce((a, b) => a + b, "")}</style><script type="module">(document.readyState === "complete" ? Promise.resolve() : new Promise(successFn => window.addEventListener("load", successFn, { "once": true }))).then(() => (${initFilterSort.toString().replace(/\n\t/g, "")})(document.getElementById("report"), Array.from(document.getElementsByTagName("fieldset")).filter(f => f.dataset.name), document.getElementsByTagName("input")));</script></head><body>${base.outerHTML}</body></html>`;

		a({ "href": URL.createObjectURL(new Blob([html], { "type": "text/html;charset=utf-8" })), "download": `backup-report-${new Date(now).toISOString()}.html` }).click();
	};

let now = 0,
	load: (path: string) => Promise<DirectoryWithChildren>;

getReportSummary().then(data => {
	now = +new Date();

	const children: Children = ["", "", ""],
		overall = new Summary(""),
		filterProject = input({ "placeholder": "Name" }),
		filterAll = input({ "id": "filterAll", "name": "filter", "type": "radio", "checked": "checked" }),
		filterR = input({ "id": "filterRed", "name": "filter", "type": "radio" }),
		filterA = input({ "id": "filterAmber", "name": "filter", "type": "radio" }),
		filterG = input({ "id": "filterGreen", "name": "filter", "type": "radio" }),
		filterB = input({ "id": "filterBlue", "name": "filter", "type": "radio" }),
		sortName = input({ "id": "sortName", "name": "sort", "type": "radio", "checked": "checked" }),
		sortWarnSize = input({ "id": "sortWarnSize", "name": "sort", "type": "radio" }),
		sortNoBackupSize = input({ "id": "sortNoBackupSize", "name": "sort", "type": "radio" }),
		sortBackupSize = input({ "id": "sortBackupSize", "name": "sort", "type": "radio" });

	for (const [dir, summary] of Object.entries(data.Summaries)) {
		const dirSummary = new ParentSummary(dir, data.BackupStatus[dir]);

		for (const rule of summary.RuleSummaries) {
			const ruleType = data.Rules[rule.ID]?.BackupType ?? -1;

			for (const user of rule.Users) {
				overall.add(ruleType, user);
				dirSummary.add(ruleType, user);
			}
		}

		for (const [child, childSummary] of [[dir, summary] as [string, ClaimedDir]].concat(Array.from(Object.entries(summary.Children)))) {
			const rules = data.Directories[child] ?? [0];

			for (const id of rules) {
				for (const stats of summary.RuleSummaries.find(v => v.ID === id)?.Users ?? []) {
					dirSummary.addChild(child, data.Rules[id] ?? {BackupType: -1, Match: ""}, stats, child === dir ? summary.ClaimedBy : childSummary.ClaimedBy ?? "", data.BackupStatus[child]);
				}
			}
		}

		children.push(dirSummary.section());
	}

	children[0] = overall.table();
	children[1] = fieldset([
		legend("Filter"),
		filterProject,
		br(),
		label({ "for": "filterAll" }, "All"), filterAll,
		label({ "for": "filterRed" }, "No backup in 6 weeks"), filterR,
		label({ "for": "filterAmber" }, "No backup in 2 weeks"), filterA,
		label({ "for": "filterGreen" }, "Backup within 2 weeks"), filterG,
		label({ "for": "filterBlue" }, "No files to backup"), filterB
	])
	children[2] = fieldset([
		legend("Sort"),
		label({ "for": "sortName" }, "Name"), sortName,
		label({ "for": "sortWarnSize" }, "Unplanned size"), sortWarnSize,
		label({ "for": "sortNoBackupSize" }, "No Backup Size"), sortNoBackupSize,
		label({ "for": "sortBackupSize" }, "Backup Size"), sortBackupSize
	])

	children[0].firstChild?.firstChild?.firstChild?.appendChild(button({ "click": download }, "Download Report"))
	initFilterSort(base, children.slice(3) as HTMLFieldSetElement[], [filterProject, filterAll, filterR, filterA, filterG, filterB, sortName, sortWarnSize, sortNoBackupSize, sortBackupSize]);
	amendNode(base, children);
});

export default Object.assign(base, {
	"init": (loadFn: (path: string) => Promise<DirectoryWithChildren>) => load = loadFn
});