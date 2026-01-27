import type { Children, Properties } from './lib/dom.js';
import { details, input, select, summary } from "./lib/html.js";
import { debouncer, setAndReturn } from './lib/utils.js';

const state = new Map<string, string>(),
	defaults = new Map<string, string>([["path", "/"], ["filterNames", "[]"], ["filterType", "none"]]),
	handlers = new Map<string, ((v: string) => void)[]>(),
	debounce = debouncer(),
	setURL = () => {
		const query: string[] = [];

		for (const [key, value] of state) {
			if (value) {
				query.push(`${encodeURIComponent(key)}=${encodeURIComponent(value)}`);
			}
		}

		const queryStr = query.join("&");

		if (queryStr !== window.location.search.slice(1)) {
			window.history.pushState(Date.now(), "", "?" + queryStr);
		}
	},
	getStateFromURL = () => {
		state.clear();

		for (const [key, value] of new URLSearchParams(window.location.search)) {
			state.set(key, value);
		}
	};

export const setState = (key: string, value: string) => {
	if (state.get(key) === value) {
		return;
	}

	if (defaults.get(key) === value) {
		state.delete(key);
	} else {
		state.set(key, value);
	}

	debounce(setURL);
},
	handleState = <T>(key: string, fn: (value: string) => T) => {
		(handlers.get(key) ?? setAndReturn(handlers, key, [])).push(fn);

		return fn(state.get(key) ?? "");
	},
	tab = (params: Properties & { "name": string, "summary": string, "open"?: boolean }, children: Children) => {
		let { name, summary: summaryText, open } = params;

		if (!name) {
			name = summaryText;

			defaults.set(name, "open");
			delete (params as any)["name"];
		} else if (open) {
			defaults.set(name, summaryText);
		}

		const d = details(params, [summary({ "click": () => queueMicrotask(() => setState(name, d.open ? "open" : summaryText)) }, summaryText), children]);

		handleState(name, v => d.open = v === summaryText || v === "" && !!open);

		return d;
	},
	inputState = (params: Properties & { "id": string }) => {
		const { id, name } = params;

		params["change"] = () => setState(id, i.value);

		const i = input(params);

		handleState(id, v => {
			i.value = v;

			i.dispatchEvent(new CustomEvent("input"));
		});

		return i;
	},
	selectState = (params: Properties & { "id": string }, children?: Children) => {
		const { id } = params;

		params["input"] = () => setState(id, JSON.stringify(Array.from(s.selectedOptions).map(opt => opt.innerText)));

		defaults.set(id, "[]");

		const s = select(params, children),
			def = JSON.stringify(Array.from(s.selectedOptions).map(opt => opt.innerText));

		handleState(id, v => {
			let changed = false;

			const selected = JSON.parse(v || def) as string[];

			for (const opt of s.options) {
				const v = selected.includes(opt.innerText);

				if (opt.selected !== v) {
					changed = true;
					opt.selected = selected.includes(opt.innerText);
				}
			}

			if (changed) {
				s.dispatchEvent(new CustomEvent("change"));
			}
		});

		return s;
	};

getStateFromURL();

window.addEventListener("popstate", () => {
	getStateFromURL();

	for (const [key, handlerFns] of handlers.entries()) {
		const v = (state.get(key) || defaults.get(key)) ?? "";

		for (const fn of handlerFns) {
			fn(v);
		}
	}
});