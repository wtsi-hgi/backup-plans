import type { ReadSummary } from './types';
import { clearNode } from './lib/dom.js';
import { table, tbody, td, th, thead, tr } from './lib/html.js';
import { formatBytes } from './lib/utils.js';

const base = tbody();

export default Object.assign(table({ "class": "prettyTable" }, [
	thead(tr([th("SubDirectory"), th("File Size"), th("File Count"), th("Last Modified")])),
	base
]), {
	"update": (path: string, data: ReadSummary, load: (path: string) => void) => clearNode(base, Object.entries(data.children).map(([name, child]) => {
		return tr({ "click": () => load(path + name) }, [
			td(name),
			td({ "title": child.size.toLocaleString() }, formatBytes(child.size)),
			td(child.files.toLocaleString()),
			td("")
		])
	}))
});