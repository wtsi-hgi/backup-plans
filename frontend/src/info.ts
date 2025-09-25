import type { DirectoryWithChildren } from './types.js';
import { clearNode, type Children } from './lib/dom.js';
import { button, table, tbody, td, th, tr } from './lib/html.js';
import { formatBytes } from './lib/utils.js';
import { claimDir, user } from './rpc.js';

const claimedByCell = td(),
	filesCell = td(),
	sizeCell = td(),
	base = table(tbody([
		tr([th("Claimed By"), claimedByCell]),
		tr([th("Files"), filesCell]),
		tr([th("Size"), sizeCell]),
	]));

export default Object.assign(base, {
	"update": (path: string, data: DirectoryWithChildren, load: (path: string) => void) => {
		clearNode(claimedByCell, data.claimedBy || button({ "click": () => claimDir(path).then(() => load(path)) }, "Claim"));
		clearNode(filesCell, data.count.toLocaleString());
		clearNode(sizeCell, { "title": data.size.toLocaleString() + " Bytes" }, formatBytes(data.size));
	}
});