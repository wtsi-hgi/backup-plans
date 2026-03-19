import { BackupType } from "../consts.js";
import type { Children } from "./dom.js";
import { button, dialog, div, wbr } from "./html.js";
import { svg, path } from "./svg.js"

const byteSizes = ["B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB", "ZiB", "YiB", "RiB", "QiB"],
	actions = BackupType.selectable.map(t => t.optionLabel());

export const formatBytes = (size: bigint) => {
	for (const suffix of byteSizes) {
		if (size < 1024n) {
			return size.toLocaleString() + suffix;
		}

		size >>= BigInt(10);
	}

	return "∞";
},
	splitLongPath = (path: string) => path.split(/(?<=\/)/).map(e => [e, wbr()]),
	secondsInDay = 86400,
	secondsInWeek = secondsInDay * 7,
	longAgo = (unix: number) => {
		const diff = Math.max(((+new Date()) / 1000) - unix, 0),
			weeks = (diff / secondsInWeek) | 0;

		if (diff < secondsInWeek) {
			const days = (diff / secondsInDay) | 0;

			if (days === 1) {
				return "1 day";
			}

			return days + " days";
		}

		if (weeks == 1) {
			return "1 week";
		}

		return weeks + " weeks";
	},
	longAgoStr = (time: string) => {
		const seconds = Math.floor(new Date(time).getTime() / 1000);
		return longAgo(seconds);
	},
	confirm = (msg: Children, doAction: () => Promise<void>) => {
		const remove = button({
			"click": () => {
				overlay.setAttribute("closedby", "none");
				remove.toggleAttribute("disabled", true);
				cancel.toggleAttribute("disabled", true);
				doAction()
					.then(() => overlay.close())
					.catch((e: Error) => {
						overlay.setAttribute("closedby", "any");
						remove.removeAttribute("disabled");
						cancel.removeAttribute("disabled");
						alert("Error: " + e.message);
					});
			}
		}, "Remove"),
			cancel = button({ "type": "button", "click": () => overlay.close() }, "Cancel"),
			overlay = document.body.appendChild(dialog({ "id": "confirm", "closedby": "any", "close": () => overlay.remove() }, [
				div(msg),
				remove, cancel
			]));

		overlay.showModal();
	},
	setAndReturn = <K, V>(m: { set: (k: K, v: V) => void }, k: K, v: V) => {
		m.set(k, v);

		return v;
	},
	action = (backupType: BackupType) => actions[+backupType] ?? "Unknown",
	stringSort = new Intl.Collator().compare,
	debouncer = <T>() => {
		let promise: Promise<T> | null;

		return (fn: () => T | Promise<T>) => {
			if (!promise) {
				let resolve: (value: T | PromiseLike<T>) => void;

				({ promise, resolve } = Promise.withResolvers<T>());

				queueMicrotask(() => {
					promise = null;

					resolve(fn());
				});
			}

			return promise;
		}
	},
	tickSVG = () => {
		return svg({ xmlns: "http://www.w3.org/2000/svg", viewBox: "0 0 12 10" }, [
			path({
				d: "M1,6 l3,3 7,-8",
				stroke: "#0f0",
				"stroke-linecap": "round",
				"stroke-linejoin": "round",
				fill: "none",
				"stroke-width": "2"
			})
		])
	};

export function createSpinner(): HTMLElement {
	const spinner = document.createElement("div");
	spinner.className = "spinner";
	return spinner;
};

export function tickSV(): SVGElement {
	return svg(
		{
			xmlns: "http://www.w3.org/2000/svg",
			viewBox: "0 0 16 10",
		}, [
		path({
			d: "M1,6 l3,3 7,-8",
			stroke: "#0f0",
			"stroke-linecap": "round",
			"stroke-linejoin": "round",
			fill: "none",
			"stroke-width": "2"
		})
	])
};

export function crossSVG(): SVGElement {
	return svg(
		{
			xmlns: "http://www.w3.org/2000/svg",
			viewBox: "0 0 12 10",
		}, [
		path({
			d: "M2,1 q5,6 8,8 M2,9 q5,-3 8,-8",
			stroke: "#f00",
			"stroke-linecap": "round",
			"stroke-linejoin": "round",
			fill: "none",
			"stroke-width": "2"
		})
	])
};