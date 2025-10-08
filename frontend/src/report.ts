import type { BackupType, Rule, SizeCount, SizeCountTime, Stats } from "./types.js";
import type { Children } from "./lib/dom.js";
import { amendNode } from "./lib/dom.js";
import { details, div, fieldset, h1, h2, legend, li, summary, table, tbody, td, th, thead, tr, ul } from "./lib/html.js";
import { action, formatBytes, longAgo, secondsInWeek, setAndReturn } from "./lib/utils.js";
import { getReportSummary } from "./rpc.js";
import { BackupIBackup, BackupNone } from "./types.js";


class Summary {
	actions: SizeCountTime[] = [];
	path: string;
	lastestMTime = 0;
	count = 0n;

	constructor(path: string) {
		this.path = path;
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
				th("Warn"),
				th("No Backup"),
				th("Temp Backup"),
				th("Backup"),
				th("Manual Backup")
			])),
			tbody([
				tr([
					th("File count"),
					td(this.actions[-1]?.count.toLocaleString() ?? "0"),
					td(this.actions[0]?.count.toLocaleString() ?? "0"),
					td(this.actions[1]?.count.toLocaleString() ?? "0"),
					td(this.actions[2]?.count.toLocaleString() ?? "0"),
					td(this.actions[3]?.count.toLocaleString() ?? "0")
				]),
				tr([
					th("File size"),
					td({ "title": (this.actions[-1]?.size ?? 0).toLocaleString() }, formatBytes(this.actions[-1]?.size ?? 0)),
					td({ "title": (this.actions[0]?.size ?? 0).toLocaleString() }, formatBytes(this.actions[0]?.size ?? 0)),
					td({ "title": (this.actions[1]?.size ?? 0).toLocaleString() }, formatBytes(this.actions[1]?.size ?? 0)),
					td({ "title": (this.actions[2]?.size ?? 0).toLocaleString() }, formatBytes(this.actions[2]?.size ?? 0)),
					td({ "title": (this.actions[3]?.size ?? 0).toLocaleString() }, formatBytes(this.actions[3]?.size ?? 0))
				])
			])
		])
	}
}

class ParentSummary extends Summary {
	claimedBy: string;
	children = new Map<string, ChildSummary>();

	constructor(path: string, claimedBy: string) {
		super(path)

		this.claimedBy = claimedBy;
	}

	addChild(child: string, rule: Rule, stats: Stats) {
		const c = this.children.get(child) ?? setAndReturn(this.children, child, new ChildSummary(child));

		c.addRule(rule.Match, rule.BackupType, stats);
	}

	section() {
		const now = (+new Date()) / 1000,
			backupTime = this.actions[BackupIBackup]?.mtime ?? 0,
			lastActivity = Math.max(now - this.lastestMTime, 0),
			lastBackupActivity = Math.max(now - backupTime),
			dt = lastBackupActivity - lastActivity;

		return fieldset({ "data-status": ((this.actions[BackupNone]?.count ?? 0n) !== this.count) ? dt < secondsInWeek ? "g" : dt < secondsInWeek * 3 ? "a" : "r" : "b" }, [
			legend(h1(this.path)),
			ul([
				this.claimedBy ? li("Requester: " + this.claimedBy) : [],
				this.actions[BackupIBackup]?.mtime ? li("Last Activity in Backed-up Set: " + longAgo(this.actions[BackupIBackup]?.mtime ?? 0)) : [],
				li("Last Activity: " + (this.lastestMTime ? longAgo(this.lastestMTime) : "--none--")),
				this.claimedBy ? li("Last Backup: unknown") : [],
				this.claimedBy ? li("Backup name: plan::" + this.path) : [],
				this.claimedBy ? li("Failures: 0 ") : []
			]),
			this.table(),
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

	addRule(match: string, action: BackupType, rule: Stats) {
		const r = this.rules.get(match) ?? setAndReturn(this.rules, match, { size: 0n, count: 0n, action });

		r.count += BigInt(rule.Files);
		r.size += BigInt(rule.Size);

		this.add(action, rule);
	}

	section() {
		return [
			this.table(),
			table({ "class": "summary" }, [
				thead(tr([th("Match"), th("Action"), th("Files"), th("Size")])),
				tbody(Array.from(this.rules.entries()).map(([match, rule]) => tr([
					td(match),
					td(action(rule.action)),
					td(rule.count.toLocaleString()),
					td({ "title": rule.size.toLocaleString() }, formatBytes(rule.size))
				])))
			])
		];
	}
}

const base = div({ "id": "report" });

getReportSummary().then(data => {
	const children: Children = [""],
		overall = new Summary("");

	for (const [dir, summary] of Object.entries(data.Summaries)) {
		const dirSummary = new ParentSummary(dir, summary.ClaimedBy);

		for (const rule of summary.RuleSummaries) {
			const ruleType = data.Rules[rule.ID]?.BackupType ?? -1;

			for (const user of rule.Users) {
				overall.add(ruleType, user);
				dirSummary.add(ruleType, user);
			}
		}

		for (const [child, ids] of Object.entries(data.Directories).filter(d => d[0].startsWith(dir))) {
			for (const id of ids) {
				for (const stats of summary.RuleSummaries.find(v => v.ID === id)?.Users ?? []) {
					dirSummary.addChild(child, data.Rules[id], stats);
				}

			}
		}

		children.push(dirSummary.section());
	}

	children[0] = overall.table();

	amendNode(base, children);
})

export default base;