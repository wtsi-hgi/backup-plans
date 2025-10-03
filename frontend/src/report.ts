import type { BackupType, SizeCountTime, Stats } from "./types.js";
import type { Children } from "./lib/dom.js";
import { amendNode } from "./lib/dom.js";
import { div, fieldset, h1, legend, li, table, tbody, td, th, thead, tr, ul } from "./lib/html.js";
import { formatBytes, longAgo, secondsInWeek } from "./lib/utils.js";
import { getReportSummary } from "./rpc.js";
import { BackupIBackup, BackupNone } from "./types.js";

class Summary {
	actions: SizeCountTime[] = [];
	path: string;
	claimedBy: string;
	lastestMTime = 0;
	count = 0n;

	constructor(path: string, claimedBy: string) {
		this.path = path;
		this.claimedBy = claimedBy;
	}

	add(action: BackupType, rule: Stats) {
		const sct = this.actions[action] ??= { size: 0n, count: 0n, mtime: 0 };

		sct.count += BigInt(rule.Files);
		sct.size += BigInt(rule.Size);
		sct.mtime = Math.max(sct.mtime, rule.MTime);

		this.lastestMTime = Math.max(this.lastestMTime, rule.MTime);
		this.count += BigInt(rule.Files);
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
			this.table()
		])
	}

	table() {
		return table({ "class": "summary" }, [
			thead(tr([
				td(),
				th("Warn"),
				th("No Backup"),
				th("Temp Backup"),
				th("Backup")
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

const base = div({ "id": "report" }),
	warnRule = {
		BackupType: -1,
		Metadata: "",
		ReviewDate: 0,
		RemoveDate: 0,
		Match: "",
		Frequency: 0
	};

getReportSummary().then(data => {
	const children: Children = [""],
		overall = new Summary("", "");

	for (const [dir, summary] of Object.entries(data.Summaries)) {
		const dirSummary = new Summary(dir, summary.ClaimedBy);

		for (const rule of summary.RuleSummaries) {
			const ruleData = data.Rules[rule.ID] ?? warnRule;

			for (const user of rule.Users) {
				overall.add(ruleData.BackupType, user);
				dirSummary.add(ruleData.BackupType, user);
			}
		}

		children.push(dirSummary.section());
	}

	children[0] = overall.table();

	amendNode(base, children);
})

export default base;