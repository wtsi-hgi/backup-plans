import type { Table } from './treemap.js';
import type { DirectoryRules, ReadSummary, Stats, Summary, Tree } from './types.js';
import { clearNode } from './lib/dom.js';
import { button, div, li, ul } from './lib/html.js';
import { generateInfo as Info } from './info.js';
import { BuildTreeMap } from './treemap.js';
import { getRules, getTree } from './rpc.js';
import { Rules } from './rules.js';


let treeLock = false;

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
	loadDirectoryChildren = (dir: string) => {
		treeLock = true;

		const entry = getCachedDirectory(dir),
			parentDir = parent(dir);

		return Promise.all([
			getRules(dir).then(storeRules(dir)),
			!entry.loadedChildren ? getTree(dir).then(storeData(dir)) : Promise.resolve(),
			!entry.loadedData ? getTree(parentDir).then(storeData(parentDir)) : Promise.resolve()
		])
			.finally(() => treeLock = false)
			.then(() => entry)
	},
	mergeRecords = (from: Record<string, Stats>, to: Record<string, Stats>) => {
		for (const [entry, data] of Object.entries(from)) {
			const existing = to[entry] ?? (to[entry] = { ID: 0, MTime: 0, Files: 0, Size: 0 });

			existing.ID = data.ID;
			existing.MTime = Math.max(existing.MTime, data.MTime);
			existing.Files += data.Files;
			existing.Size += data.Size;
		}
	},
	loadTree = (path: string) => {
		loadDirectoryChildren(path).then(data => {
			const entries: Table = [],
				breadcrumbs = path.slice(1, -1).split("/");

			for (const [dir, child] of Object.entries(data.children)) {
				entries.push({
					"name": dir.replace("/", ""),
					"value": Number(child.size),
					"backgroundColour": "#ff8888",
					"onclick": () => {
						if (!treeLock) {
							loadTree(path + dir);
						}
					},
					"onmouseover": () => { },
					"noauth": false
				})
			}

			entries.sort((a, b) => b.value - a.value);

			clearNode(base, [
				ul({ "id": "breadcrumbs" }, [
					li(button({ "click": () => loadTree("/"), "title": "Jump To: /" }, "/")),
					breadcrumbs.slice(0, -1).map((part, n) => li(button({ "click": () => loadTree("/" + breadcrumbs.slice(0, n + 1).join("/") + "/"), "title": `Jump To: ${part}` }, part))),
					breadcrumbs[0] ? li(breadcrumbs.at(-1)) : []
				]),
				BuildTreeMap(entries, 500, 400, false, () => { }),
				Info(path, data, loadTree),
				Rules(path, data, loadTree)
			]);
		});
	};

export const base = div();

getCachedDirectory("/").loadedData = true;

loadDirectoryChildren("/").then(() => {
	const root = getCachedDirectory("/")!;

	for (const child of Object.values(root.children)) {
		mergeRecords(child.Users, root.Users);
		mergeRecords(child.Groups, root.Groups);
	}

	[root.files, root.size] = countStats(root.Users);

	loadTree("/");
});