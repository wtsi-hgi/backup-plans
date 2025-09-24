import { BackupWarn, type Directory, type DirectoryWithChildren, type DirSummary, type Rule, type Rules, type RuleStats, type RuleSummary, type SizeCountTime } from "./types.js";
import { getTree, passDirClaim } from "./rpc.js";
import { feColorMatrix } from "./lib/svg.js";

// type SizeCount = {
// 	size: number;
// 	count: number;
// }

// type RuleStats = Rule & Stats;

// type Directory = {
// 	files: number;
// 	size: number;
// 	actions: SizeCount[];
// 	users: string[];
// 	groups: string[];
// 	rules: Record<string, RuleStats[]>;
// }

// type DirectoryWithChildren = Directory & {
// 	children: Record<string, Directory>;
// }

// User/Group list
// Warn/Backup/Temp Backup/Manual Backup counts/size
// Summarise rules

type RulesWithDirs = Record<number, Rule & { dir: string }>;

const all = function* (rs: RuleSummary) {
	for (const r of rs.Users) {
		yield r;
	}
},
	user = (uid: number) => function* (rs: RuleSummary) {
		for (const r of rs.Users) {
			if (r.ID === uid) {
				return r;
			}
		}
	},
	group = (gid: number) => function* (rs: RuleSummary) {
		for (const r of rs.Groups) {
			if (r.ID === gid) {
				return r;
			}
		}
	},
	addCountSizeTime = (s: SizeCountTime, size: bigint, count: bigint, time: number) => {
		s.size += size;
		s.count += count;
		s.mtime = Math.max(s.mtime, time);
	},
	summarise = (tree: DirSummary, d: Directory, rules: RulesWithDirs, filter = all) => {
		for (const rs of tree.RuleSummaries) {
			const rule = Object.assign(rules[rs.ID], {
				"count": 0n,
				"size": 0n,
				"mtime": 0
			}),
				action = (d.actions[rule.BackupType] ??= {
					"count": 0n,
					"size": 0n,
					"mtime": 0
				});

			for (const stats of filter(rs)) {
				addCountSizeTime(d, BigInt(stats.Size), BigInt(stats.Files), stats.MTime);
				addCountSizeTime(rule, BigInt(stats.Size), BigInt(stats.Files), stats.MTime);
				addCountSizeTime(action, BigInt(stats.Size), BigInt(stats.Files), stats.MTime);
			}

			(d.rules[rule.dir] ??= []).push(rule);
		}
	}

export default (path: string) => getTree(path).then(data => {
	const d: DirectoryWithChildren = {
		"claimedBy": data.ClaimedBy,
		"count": 0n,
		"size": 0n,
		"mtime": 0,
		"actions": [],
		"users": [],
		"groups": [],
		"rules": {},
		"children": {}
	},
		rules = Object.entries(data.Rules)
			.map(([dir, rules]) => Object.entries(rules).map(([id, rule]) => Object.assign(rule, { id, dir })))
			.flat()
			.reduce((c, r) => (c[r.id as unknown as number] = r, c), {} as RulesWithDirs);

	rules[0] = {
		"BackupType": BackupWarn,
		"Metadata": "",
		"ReviewDate": 0,
		"RemoveDate": 0,
		"Match": "*",
		"Frequency": 0,
		"dir": ""
	};


	for (const [name, child] of Object.entries(data.Children)) {
		const e: Directory = {
			"count": 0n,
			"size": 0n,
			"mtime": 0,
			"actions": [],
			"users": [],
			"groups": [],
			"rules": {}
		}

		summarise(child, e, rules);

		d.children[name] = e;
	};

	summarise(data, d, rules);

	return d;
});