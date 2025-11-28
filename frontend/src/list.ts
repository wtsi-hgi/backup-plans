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
	"update": (path: string, data: DirectoryWithChildren, load: (path: string) => void) => clearNode(base, Object.entries(data.children).map(([name, child]) => {
		return tr({ "style": child.unauthorised ? "cursor: not-allowed;" : "", "click": () => child.unauthorised || load(path + name) }, [
			td(name),
			td({ "title": child.size.toLocaleString() }, formatBytes(child.size)),
			td(child.count.toLocaleString()),
			td(dateFormat.format(new Date(child.mtime * 1000)))
		])
	}))
});