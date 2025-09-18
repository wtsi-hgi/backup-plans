import type { Children } from './lib/dom.js';
import type { ReadSummary } from './types.js';
import { button, table, tbody, td, th, tr } from './lib/html.js';
import { claimDir, user } from './rpc.js';

const byteSizes = ["B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB", "ZiB", "YiB", "RiB", "QiB"],
	formatBytes = (size: bigint) => {
		for (const suffix of byteSizes) {
			if (size < 1024n) {
				return size.toLocaleString() + suffix;
			}

			size >>= BigInt(10);
		}

		return "∞";
	};

export const generateInfo = (path: string, data: ReadSummary, loadTree: (path: string) => void): Children => {
	return [
		table(tbody([
			tr([th("Claimed By"), td(data.rules.ClaimedBy || button({ "click": () => claimDir(path).then(() => loadTree(path)) }, "Claim"))]),
			tr([th("Files"), td(data.files.toLocaleString())]),
			tr([th("Size"), td({ "title": data.size.toLocaleString() + " Bytes" }, formatBytes(data.size))]),
		]))
	];
}