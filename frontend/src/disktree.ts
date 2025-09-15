import type { Stats, Summary } from './rpc.js';
import type { Table } from './treemap.js';
import { button, div, li, table, tbody, td, th, tr, ul } from './lib/html.js';
import { getTree } from './rpc.js';
import { BuildTreeMap } from './treemap.js';
import { clearNode } from './lib/dom.js';

type ReadSummary = Summary & {
	loaded: boolean;
	children: Record<string, ReadSummary>;
	files: number;
	size: bigint;
}

let treeLock = false;

const directories = new Map<string, ReadSummary>(),
	countStats = (s: Record<string, Stats>) => Object.values(s).reduce((c, n) => (c[0] += n.Files, c[1] += BigInt(n.Size), c), [0, 0n] as [number, bigint]),
	loadDirectoryChildren = (dir: string) => {
		const cached = directories.get(dir);

		if (cached?.loaded) {
			return Promise.resolve(cached);
		}

		treeLock = true;

		return getTree(dir).then(data => {
			treeLock = false;

			for (const [path, child] of Object.entries(data)) {
				const thisPath = dir + path,
					existing = directories.get(thisPath),
					[files, size] = countStats(child.Users);

				directories.set(thisPath, Object.assign(child, {
					"children": existing?.children ?? {},
					"loaded": existing?.loaded ?? false,
					"files": files,
					"size": BigInt(size),
				}));
			}

			if (cached) {
				cached.children = data as Record<string, ReadSummary>;
				cached.loaded = true;

				return cached;
			}

			const dirData: ReadSummary = {
				loaded: true,
				children: data as Record<string, ReadSummary>,
				User: "",
				Group: "",
				Users: {},
				Groups: {},
				files: 0,
				size: 0n
			};

			directories.set(dir, dirData);


			return dirData;
		})
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
	byteSizes = ["B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB", "ZiB", "YiB", "RiB", "QiB"],
	formatBytes = (size: bigint) => {
		for (const suffix of byteSizes) {
			console.log(size)
			if (size < 1024) {
				return size.toLocaleString() + suffix;
			}

			size >>= BigInt(10);
		}

		return "∞";
	},
	loadTree = (path: string) => {
		loadDirectoryChildren(path).then(data => {
			const entries: Table = [],
				breadcrumbs = path.slice(1, -1).split("/"),
				info = table(tbody([
					tr([th("Files"), td(data.files.toLocaleString())]),
					tr([th("Size"), td({ "title": data.size.toLocaleString() + " Bytes" }, formatBytes(data.size))]),
				]));

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
				info
			]);
		})
	};

export const base = div();

loadDirectoryChildren("/").then(() => {
	const root = directories.get("/")!;

	for (const child of Object.values(root.children)) {
		mergeRecords(child.Users, root.Users);
		mergeRecords(child.Groups, root.Groups);
	}

	[root.files, root.size] = countStats(root.Users);

	loadTree("/");
});
