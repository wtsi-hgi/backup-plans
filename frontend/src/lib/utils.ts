import type { Children } from "./dom.js";
import { button, dialog, div } from "./html.js";

const byteSizes = ["B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB", "ZiB", "YiB", "RiB", "QiB"],
	actions = ["No Backup", "IBackup", "Manual Backup"];

export const formatBytes = (size: bigint) => {
	for (const suffix of byteSizes) {
		if (size < 1024n) {
			return size.toLocaleString() + suffix;
		}

		size >>= BigInt(10);
	}

	return "∞";
},
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
	action = (backupType: number) => actions[backupType] ?? "Unknown",
	stringSort = new Intl.Collator().compare;