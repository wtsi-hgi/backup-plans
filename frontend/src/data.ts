import type { Directory, DirectoryWithChildren, DirSummary, Rule, RuleSummary, SizeCountTime, Stats } from './types.js';
import { BackupWarn } from "./types.js";
import { filter } from './filter.js';
import { getTree } from "./rpc.js";


type RulesWithDirs = Record<number, Rule & { dir: string }>;

const all = function* (rs: RuleSummary) {
	for (const r of rs.Users) {
		yield r;
	}
},
	users = (users: string[]) => function* (rs: RuleSummary) {
		for (const r of rs.Users) {
			if (users.includes(r.Name)) {
				yield r;
			}
		}
	},
	groups = (groups: string[]) => function* (rs: RuleSummary) {
		for (const r of rs.Groups) {
			if (groups.includes(r.Name)) {
				yield r;
			}
		}
	},
	addCountSizeTime = (s: SizeCountTime, size: bigint, count: bigint, time: number) => {
		s.size += size;
		s.count += count;
		s.mtime = Math.max(s.mtime, time);
	},
	summarise = (tree: DirSummary, d: Directory, rules: RulesWithDirs, filter: (rs: RuleSummary) => Generator<Stats>) => {
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
		"users": Array.from(data.RuleSummaries.map(rs => rs.Users).map(u => u.map(u => u.Name)).flat().reduce((s, u) => (s.add(u), s), new Set<string>()).keys()).sort(),
		"groups": Array.from(data.RuleSummaries.map(rs => rs.Groups).map(g => g.map(g => g.Name)).flat().reduce((s, u) => (s.add(u), s), new Set<string>()).keys()).sort(),
		"rules": { [path]: [] },
		"children": {},
		"unauthorised": false,
		"canClaim": data.CanClaim
	},
		rules = Object.entries(data.Rules)
			.map(([dir, rules]) => Object.entries(rules).map(([id, rule]) => Object.assign(rule, { id, dir })))
			.flat()
			.reduce((c, r) => (c[r.id as unknown as number] = r, c), {} as RulesWithDirs),
		filterFn = filter.type === "users" ? users(filter.names) : filter.type === "groups" ? groups(filter.names) : all;

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
			"users": Array.from(child.RuleSummaries.map(rs => rs.Users).map(u => u.map(u => u.Name)).flat().reduce((s, u) => (s.add(u), s), new Set<string>()).keys()).sort(),
			"groups": Array.from(child.RuleSummaries.map(rs => rs.Groups).map(g => g.map(g => g.Name)).flat().reduce((s, u) => (s.add(u), s), new Set<string>()).keys()).sort(),
			"rules": {},
			"unauthorised": data.Unauthorised.includes(name)
		}

		summarise(child, e, rules, filterFn);

		d.children[name] = e;
	};

	summarise(data, d, rules, filterFn);

	for (const [_, rule] of Object.entries(data.Rules[path] ?? []) as unknown[] as [number, Rule][]) {
		if (!d.rules[path].some(e => e.Match === rule.Match)) {
			d.rules[path].push(Object.assign(rule, {
				"count": 0n,
				"size": 0n,
				"mtime": 0
			}));
		}
	}

	return d;
});