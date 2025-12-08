import type { Directory, DirectoryWithChildren } from './types.js';
import type { Children } from './lib/dom.js';
import { clearNode } from './lib/dom.js';
import { br, details, div, label, option, select, summary } from './lib/html.js';
import { rect, svg, text, use } from './lib/svg.js';
import { BackupType } from './consts.js';
import UserStats, { diskTreeHover } from './userstats.js';

export type Entry = {
	name: string;
	value: number;
	colour?: string;
	backgroundColour?: string;
	onclick?: () => void;
	onmouseover?: () => void;
	noauth: boolean;
}

export type Table = Entry[];

type Box = {
	left: number;
	top: number;
	right: number;
	bottom: number;
}

type BoxParams = {
	entry: Entry;
	top: number;
	left: number;
	colWidth: number;
	rowHeight: number;
	minScale: number;
	bbox: { width: number; height: number; depth: number };
}

const phi = (1 + Math.sqrt(5)) / 2,
	underhangs = ['g', 'j', 'p', 'q', 'y'],
	smallLetters = /^[acemnorsuvwxz]+$/,
	DirBox = ({ entry, top, left, colWidth, rowHeight, minScale, bbox }: BoxParams): Children => [
		rect({
			"x": left + "",
			"y": top + "",
			"width": colWidth + "",
			"height": rowHeight + "",
			"tabIndex": "0",
			"aria-label": entry.name + (entry.onclick ? "" : entry.noauth ? "; No authorisation to view" : "; No children with current filter"),
			"style": "stroke: currentColor; --fill: " + entry.backgroundColour,
			"class": entry.onclick ? "hasClick box" : "box",
			"click": (e: MouseEvent) => {
				if (e.button !== 0) {
					return;
				}

				entry.onclick?.();
			},
			"keypress": entry.onclick ? (e: KeyboardEvent) => {
				if (e.key === "Enter") {
					entry.onclick?.();
				}
			} : () => { },
			"mouseover": entry.onmouseover ?? (() => { }),
		}),
		entry.onclick ? [] :
			entry.noauth ?
				use({
					"x": left + (colWidth - bbox.width * minScale) / 2 + "",
					"y": top + (rowHeight - minScale * 0.5) / 2 + "",
					"href": "#lock",
					"width": "0.5em",
					"height": "0.5em",
					"style": `color: #000; font-size: ${minScale}px;`
				}) :
				use({
					"x": left + (colWidth - bbox.width * minScale) / 2 + "",
					"y": top + (rowHeight - minScale * 0.25) / 2 + "",
					"href": "#emptyDirectory",
					"width": "0.5em",
					"height": "0.3846em",
					"style": `color: "#000", fontSize: ${minScale * 0.9}px`
				}),
		text({
			"font-size": minScale + "",
			"font-family": "",
			"x": (entry.noauth ? minScale * 0.225 : 0) + left + colWidth / 2 + "",
			"y": top + rowHeight / 2 + "",
			"text-anchor": "middle",
			"fill": entry.colour ?? "#000",
			"dominant-baseline": underhangs.some(u => entry.name.includes(u)) || smallLetters.test(entry.name) ? "middle" : "central"
		}, entry.name)
	],
	buildTree = (table: Table, box: Box) => {
		let lastFontSize = Infinity,
			remainingTotal = 0,
			pos = 0;

		for (const e of table) {
			remainingTotal += e.value;
		}

		const toRet: Children = [];

		while (box.right - box.left >= 1 && box.bottom - box.top >= 1) {
			const isRow = (box.right - box.left) / (box.bottom - box.top) < phi || pos === 0,
				boxWidth = box.right - box.left,
				boxHeight = box.bottom - box.top;

			let total = table[pos].value,
				split = pos + 1;
			const totalRatio = total / remainingTotal;
			let lastDR = phi - boxWidth * (isRow ? 1 : totalRatio) / (boxHeight * (isRow ? totalRatio : 1)),
				d = isRow ? box.left : box.top;

			for (let i = split; i < table.length; i++) {
				const { value } = table[i],
					nextTotal = total + value,
					rowHeight = boxHeight * (isRow ? nextTotal / remainingTotal : value / nextTotal),
					colWidth = boxWidth * (isRow ? value / nextTotal : nextTotal / remainingTotal),
					dRatio = phi - colWidth / rowHeight;

				if ((isRow || lastDR < 0) && Math.abs(dRatio) > Math.abs(lastDR)) {
					break;
				}

				lastDR = dRatio;
				split++;
				total = nextTotal;
			}

			for (let i = pos; i < split; i++) {
				const entry = table[i],
					top = isRow ? box.top : d,
					left = isRow ? d : box.left,
					colWidth = isRow ? boxWidth * entry.value / total : boxWidth * total / remainingTotal,
					rowHeight = isRow ? boxHeight * total / remainingTotal :
						boxHeight * entry.value / total,
					bbox = getTextBB((entry.onclick ? "" : "W") + entry.name),
					xScale = colWidth / bbox.width,
					yScale = 0.9 * rowHeight / (bbox.height + bbox.depth),
					minScale = lastFontSize = Math.min(xScale, yScale, lastFontSize);

				d += isRow ? colWidth : rowHeight;

				toRet.push(DirBox({ entry, top, left, colWidth, rowHeight, minScale: minScale * 0.75, bbox }));
			}

			if (isRow) {
				box.top += boxHeight * total / remainingTotal;
			} else {
				box.left += boxWidth * total / remainingTotal;
			}

			pos = split;
			remainingTotal -= total;
		}

		return toRet;
	},
	font = "\"Helvetica Neue\", Helvetica, Arial, sans-serif",
	getTextBB = (() => {
		const ctx = document.createElement("canvas").getContext("2d"),
			fontSize = 1000; // Fix for WebKit/Blink bug around font rendering at small sizes.

		if (!ctx) {
			return () => ({ "width": 1, "height": 1, "depth": 1 });
		}

		ctx.font = `${fontSize}px ${font}`;

		return (text: string) => {
			const { width = fontSize, actualBoundingBoxAscent: height = fontSize, actualBoundingBoxDescent: depth = 0 } = ctx.measureText(text) ?? { "width": fontSize, "actualBoundingBoxAscent": fontSize, "actualBoundingBoxDescent": 0 };

			return { width: width / fontSize, height: height / fontSize, depth: depth / fontSize };
		};
	})(),
	maxTableEntries = 1000,
	buildTreeMap = (table: Table | null, width: number, height: number, noAuth?: boolean, onmouseout?: (e: MouseEvent) => void) => {
		if (table === null) {
			return [] as Children;
		}

		const filteredTable: Table = [],
			box: Box = {
				"left": 0,
				"top": 0,
				"right": width,
				"bottom": height
			};

		for (const entry of table) {
			if (entry.value > 0) {
				if (filteredTable.length === maxTableEntries) {
					break;
				}

				filteredTable.push(entry);
			}
		}

		if (filteredTable.length === 0) {
			return svg({ "class": "treeMap", "width": width + "", "height": height + "", "viewBox": `0 0 ${width} ${height}` }, [
				rect({ "width": "100%", "height": "100%", "stroke": "currentColor", "style": `fill: var(--background)` }),
				noAuth ?
					use({ "tab-index": "0", "aria-label": "Not authorised to access this directory", "href": "#lock", "height": "150", "transform": `translate(0 ${(height - 200) / 2})` })
					:
					use({ "tab-index": "0", "aria-label": "Directory has no children with current filter", "href": "#emptyDirectory", "height": "150", "transform": `translate(0 ${(height - 200) / 2})` }),
			])
		}

		return svg({ "class": "treeMap", "width": width + "", "height": height + "", "viewBox": `0 0 ${width} ${height}`, "mouseout": onmouseout ?? (() => { }) },
			buildTree(filteredTable, box)
		)
	},
	secondsInSevenYears = 7 * 365 * 86400,
	colourFns = [
		(dir: Directory) => dir.size ? 100 * Number(dir.size - (dir.actions[+BackupType.BackupWarn]?.size ?? 0n)) / Number(dir.size) : 100,
		(dir: Directory) => dir.count ? 100 * Number(dir.count - (dir.actions[+BackupType.BackupWarn]?.count ?? 0n)) / Number(dir.count) : 100,
		(dir: Directory) => dir.size ? 100 * Number(dir.actions[+BackupType.BackupIBackup]?.size ?? 0n) / Number(dir.size) : 100,
		(dir: Directory) => dir.count ? 100 * Number(dir.actions[+BackupType.BackupIBackup]?.count ?? 0n) / Number(dir.count) : 100,
		(dir: Directory) => dir.size ? 100 * Number(dir.actions[+BackupType.BackupNone]?.size ?? 0n) / Number(dir.size) : 100,
		(dir: Directory) => dir.count ? 100 * Number(dir.actions[+BackupType.BackupNone]?.count ?? 0n) / Number(dir.count) : 100,
		(dir: Directory) => Math.max(0, Math.min(100, 100 * (dir.mtime - (+new Date() / 1000) + secondsInSevenYears) / secondsInSevenYears))
	],
	areaFns = [
		(dir: Directory) => Number(dir.actions[+BackupType.BackupWarn]?.size ?? 0),
		(dir: Directory) => Number(dir.actions[+BackupType.BackupWarn]?.count ?? 0),
		(dir: Directory) => Number(dir.actions[+BackupType.BackupIBackup]?.size ?? 0),
		(dir: Directory) => Number(dir.actions[+BackupType.BackupIBackup]?.count ?? 0),
		(dir: Directory) => Number(dir.actions[+BackupType.BackupNone]?.size ?? 0),
		(dir: Directory) => Number(dir.actions[+BackupType.BackupNone]?.count ?? 0),
		(dir: Directory) => Number(dir.size),
		(dir: Directory) => Number(dir.count)
	],
	options = details({ "id": "treeOptions" }, [
		summary("View Options"),
		label({ "for": "colourBy" }, "Colour By"),
		select({
			"id": "colourBy", "change": function (this: HTMLSelectElement) {
				colourFn = Math.max(0, Math.min(colourFns.length - 1, parseInt(this.value) || 0));
				reload();
			}
		}, [
			option({ "value": "0" }, "Planned Size %"),
			option({ "value": "1" }, "Planned Files %"),
			option({ "value": "2" }, "Backup Size %"),
			option({ "value": "3" }, "Backup Files %"),
			option({ "value": "4" }, "No Backup Size %"),
			option({ "value": "5" }, "No Backup Files %"),
			option({ "value": "6" }, "Modified Time")
		]),
		br(),
		div({ "id": "spectrum" }),
		br(),
		label({ "for": "areaRepresents" }, "Area Represents"),
		select({
			"id": "areaRepresents", "change": function (this: HTMLSelectElement) {
				areaFn = Math.max(0, Math.min(areaFns.length - 1, parseInt(this.value) || 0));
				reload();
			}
		}, [
			option({ "value": "0" }, "Unplanned Size"),
			option({ "value": "1" }, "Unplanned Files"),
			option({ "value": "2" }, "Backup Size"),
			option({ "value": "3" }, "Backup Files"),
			option({ "value": "4" }, "No Backup Size"),
			option({ "value": "5" }, "No Backup Files"),
			option({ "value": "6" }, "Total Size"),
			option({ "value": "7" }, "Total Files"),
		])
	]),
	svgBase = div(),
	base = div({ "id": "tree" }, [
		svgBase,
		options
	])

export let render = () => { };

let areaFn = 0,
	colourFn = 0,
	reload = () => { },
	lastWidth = 0,
	lastHeight = 0;

new ResizeObserver(() => render()).observe(svgBase);

export default Object.assign(base, {
	"update": (path: string, data: DirectoryWithChildren, load: (path: string) => void) => {
		const entries: Table = [];

		for (const [dir, child] of Object.entries(data.children)) {
			entries.push({
				"name": dir.replace("/", ""),
				"value": areaFns[areaFn](child),
				"backgroundColour": colourFns[colourFn](child) + "",
				"onclick": child.unauthorised ? undefined : () => load(path + dir),
				"onmouseover": () => diskTreeHover(dir),
				"noauth": child.unauthorised
			})
		}

		entries.sort((a, b) => b.value - a.value);

		render = () => clearNode(svgBase, buildTreeMap(entries, lastWidth = svgBase.clientWidth || lastWidth, lastHeight = svgBase.clientHeight || lastHeight, data.unauthorised, () => diskTreeHover("")));

		render();

		reload = () => load(path);
	}
});