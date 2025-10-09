import type { DirectoryWithChildren } from "./types.js";
import { clearNode } from "./lib/dom.js";
import { a, button, caption, div, h2, table, tbody, td, th, thead, tr } from "./lib/html.js";
import { svg, title, use } from "./lib/svg.js";
import { action, formatBytes, stringSort } from "./lib/utils.js";

const base = div({ "class": "affectingRules" });

export default Object.assign(base, {
	"update": (path: string, data: DirectoryWithChildren, load: (path: string) => void) => {
		clearNode(base, !Object.entries(data.rules).some(([dir]) => dir && dir !== path) ? [] : [
			h2("Rules affecting files within this directory tree"),
			Object.entries(data.rules).toSorted(([dira], [dirb]) => stringSort(dira, dirb)).map(([dir, rules]) => dir && dir !== path && rules.length ? table({ "class": "summary" }, [
				caption(h2([
					dir,
					button({ "click": () => load(dir) }, svg([
						title("Go to"),
						use({ "href": "#goto" })
					]))
				])),
				thead(tr([th("Match"), th("Action"), th("Files"), th("Size")])),
				tbody(rules.map(rule => tr([
					td(rule.Match),
					td(action(rule.BackupType)),
					td(rule.count.toLocaleString()),
					td({ "title": rule.size.toLocaleString() }, formatBytes(rule.size))
				])))
			]) : [])
		])
	}
});