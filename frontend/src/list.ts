import type { DirectoryWithChildren } from './types.js';
import { clearNode } from './lib/dom.js';
import { table, tbody, td, th, thead, tr, div } from './lib/html.js';
import { formatBytes } from './lib/utils.js';
import { onHover } from './userstats.js';
import { load, registerLoader } from './load.js';

registerLoader((path: string, data: DirectoryWithChildren) => {
	const entries = Object.entries(data.children).filter(([, child]) => child.count !== 0n);
	if (entries.every(([name]) => isNumber(getDirFromEntry(name)))) {
		entries.sort(([a], [b]) => Number(getDirFromEntry(a)) - Number(getDirFromEntry(b)));
	}

	clearNode(base, entries.map(([name, child]) => {
		return tr(
			{
				"style": child.unauthorised ? "cursor: not-allowed;" : "",
				"click": () => child.unauthorised || load(path + name),
				"mouseover": () => onHover(name),
				"mouseout": () => onHover(""),
			},
			[
				td(name),
				td({ "title": child.size.toLocaleString() }, formatBytes(child.size)),
				td(child.count.toLocaleString()),
				td(dateFormat.format(new Date(child.mtime * 1000)))
			])
	}))
	updateScrollWidth();
});

const base = tbody(),
	dateFormat = new Intl.DateTimeFormat('en-GB', { timeZone: 'Europe/London', year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' }),
	container = div({ class: "prettyTableContainer" }, [
		table({ "class": "prettyTable", "id": "dirlist" }, [
			thead(tr([
				th("SubDirectory"),
				th("File Size"),
				th("File Count"),
				th("Last Modified")
			])),
			base
		])
	]),
	resizeObserver = new ResizeObserver(updateScrollWidth);

resizeObserver.observe(container);

function isNumber(n: string): boolean {
	return !isNaN(parseFloat(String(n))) && isFinite(Number(n));
}

function getDirFromEntry(name: string): string {
	return name.slice(0, name.length - 1);
}

function updateScrollWidth() {
	const hasScrollbar = container.scrollHeight > container.clientHeight;
	const width = hasScrollbar ? container.offsetWidth - container.clientWidth : 0;
	container.style.setProperty('--scrollwidth', width + 'px');
}

export default container;