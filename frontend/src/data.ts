import type { DirectoryRules, ReadSummary, Stats, Tree } from "./types.js";
import { getRules, getTree } from "./rpc.js";

const directories = new Map<string, ReadSummary>(),
	setAndReturn = <K, V>(m: { set: (k: K, v: V) => void }, k: K, v: V) => {
		m.set(k, v);

		return v;
	},
	countStats = (s: Record<string, Stats>) => Object.values(s).reduce((c, n) => (c[0] += n.Files, c[1] += BigInt(n.Size), c), [0, 0n] as [number, bigint]),
	getCachedDirectory = (dir: string) => directories.get(dir) ?? setAndReturn(directories, dir, {
		"loadedData": false,
		"loadedChildren": false,
		"children": {},
		"files": 0,
		"size": 0n,
		"User": "",
		"Group": "",
		"Users": {},
		"Groups": {},
		"rules": {
			"ClaimedBy": "",
			"Rules": {}
		}
	}),
	storeData = (dir: string) => (data: Tree) => {
		const entry = getCachedDirectory(dir);

		for (const [path, child] of Object.entries(data)) {
			const [files, size] = countStats(child.Users);

			data[path] = Object.assign(getCachedDirectory(dir + path), {
				"loadedData": true,
				"User": child.User,
				"Group": child.Group,
				"Users": child.Users,
				"Groups": child.Groups,
				"files": files,
				"size": size,
			});
		}

		entry.loadedChildren = true;
		entry.children = data;
	},
	storeRules = (dir: string) => (rules: DirectoryRules) => {
		const entry = getCachedDirectory(dir);

		entry.rules = Object.assign(entry.rules, rules);
	},
	parent = (dir: string) => dir.slice(0, dir.slice(0, -1).lastIndexOf("/") + 1),
	queue = (() => {
		let p = Promise.resolve();

		return (fn: () => Promise<any>) => p = p.finally(fn);
	})(),
	mergeRecords = (from: Record<string, Stats>, to: Record<string, Stats>) => {
		for (const [entry, data] of Object.entries(from)) {
			const existing = to[entry] ?? (to[entry] = { ID: 0, MTime: 0, Files: 0, Size: 0 });

			existing.ID = data.ID;
			existing.MTime = Math.max(existing.MTime, data.MTime);
			existing.Files += data.Files;
			existing.Size += data.Size;
		}
	},
	loadDirectoryChildren = (dir: string) => {
		const entry = getCachedDirectory(dir),
			parentDir = parent(dir);

		return queue(() => Promise.all([
			getRules(dir).then(storeRules(dir)),
			!entry.loadedChildren ? getTree(dir).then(storeData(dir)) : Promise.resolve(),
			!entry.loadedData ? getTree(parentDir).then(storeData(parentDir)) : Promise.resolve()
		])).then(() => entry)
	}

export default loadDirectoryChildren;

getCachedDirectory("/").loadedData = true;

loadDirectoryChildren("/").then(() => {
	const root = getCachedDirectory("/")!;

	for (const child of Object.values(root.children)) {
		mergeRecords(child.Users, root.Users);
		mergeRecords(child.Groups, root.Groups);
	}

	[root.files, root.size] = countStats(root.Users);
});