import { clearNode, type Children } from './lib/dom.js';
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
	},
	[claimedByCell, filesCell, sizeCell] = [td(), td(), td()],
	base = table(tbody([
		tr([th("Claimed By"), claimedByCell]),
		tr([th("Files"), filesCell]),
		tr([th("Size"), sizeCell]),
	]));

export default Object.assign(base, {
	"update": (path: string, data: ReadSummary, load: (path: string) => void) => {
		clearNode(claimedByCell, data.rules.ClaimedBy || button({ "click": () => claimDir(path).then(() => load(path)) }, "Claim"));
		clearNode(filesCell, data.files.toLocaleString());
		clearNode(sizeCell, { "title": data.size.toLocaleString() + " Bytes" }, formatBytes(data.size));
	}
});