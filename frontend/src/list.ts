import type { DirectoryWithChildren } from './types.js';
import { clearNode } from './lib/dom.js';
import { table, tbody, td, th, thead, tr, div } from './lib/html.js';
import { formatBytes } from './lib/utils.js';

const base = tbody(),
	dateFormat = new Intl.DateTimeFormat('en-GB', { timeZone: 'Europe/London', year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' });

export default Object.assign(
	div({ class: "prettyTableContainer" }, [
		table({ "class": "prettyTable", "id": "dirlist" }, [
			thead(tr([
				th("SubDirectory"),
				th("File Size"),
				th("File Count"),
				th("Last Modified")
			])),
			base
		])
	]), {
	"update": (path: string, data: DirectoryWithChildren, load: (path: string) => void) => {
		const entries = Object.entries(data.children);
		if (entries.every(entry => isNumber(entry[0].slice(0, entry[0].length - 1)))) {
			entries.sort((a, b) => Number(a[0].slice(0, a[0].length - 1)) - Number(b[0].slice(0, b[0].length - 1)));
		}
		clearNode(base, entries.map(([name, child]) => {
			return tr({ "style": child.unauthorised ? "cursor: not-allowed;" : "", "click": () => child.unauthorised || load(path + name) }, [
				td(name),
				td({ "title": child.size.toLocaleString() }, formatBytes(child.size)),
				td(child.count.toLocaleString()),
				td(dateFormat.format(new Date(child.mtime * 1000)))
			])
		}))
	}
});

function isNumber(n: string): boolean {
	return !isNaN(parseFloat(String(n))) && isFinite(Number(n));
}
