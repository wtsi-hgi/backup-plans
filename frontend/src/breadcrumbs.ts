import { clearNode } from "./lib/dom.js";
import { button, li, ul } from "./lib/html.js";
import { svg, title, use } from "./lib/svg.js";

const base = ul({ "id": "breadcrumbs" });

export default Object.assign(base, {
	"update": (path: string, load: (path: string) => void) => {
		const breadcrumbs = path.slice(1, -1).split("/");

		clearNode(base, [
			li(button({ "click": () => load("/"), "title": "Jump To: /" }, "/")),
			breadcrumbs.slice(0, -1).map((part, n) => li(button({ "click": () => load("/" + breadcrumbs.slice(0, n + 1).join("/") + "/"), "title": `Jump To: ${part}` }, part))),
			breadcrumbs[0] ? li(breadcrumbs.at(-1)) : [],
			button({
				"id": "copy", "click": function (this: HTMLButtonElement) {
					navigator.clipboard.writeText(path);
					this.classList.toggle("highlight", true);
					setTimeout(() => this.classList.toggle("highlight", false), 200);
				}
			}, svg([
				title("Copy Path"),
				use({ "href": "#copy" })
			]))
		]);
	}
});