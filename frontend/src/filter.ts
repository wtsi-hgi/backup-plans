import type { DirectoryWithChildren } from './types';
import { br, button, details, div, input, option, select, summary } from './lib/html.js';

const userSelect = select({ "multiple": "multiple" }),
	groupSelect = select({ "multiple": "multiple" }),
	buildFilter = (s: HTMLSelectElement) => input({
		"placeholder": "Filter", "input": function (this: HTMLInputElement) {
			for (const child of s.children as unknown as HTMLOptionElement[]) {
				child.style.display = child.innerText.includes(this.value) ? "" : "none"
			}
		}
	}),
	userFilter = buildFilter(userSelect),
	groupFilter = buildFilter(groupSelect),
	getValues = (select: HTMLSelectElement) => Array.from(select.options).filter(opt => opt.selected).map(opt => opt.innerText),
	setFilter = (type: string, select: HTMLSelectElement) => {
		const names = getValues(select);

		if (names.length === 0) {
			filter["type"] = "none";
		} else {
			filter["type"] = type;
			filter["names"] = names;
		}

		loadFiltered();
	},
	clearFilter = (input: HTMLInputElement, select: HTMLSelectElement) => {
		for (const opt of select.options) {
			opt.selected = false;
			opt.style.display = "";
		}

		filter["type"] = "none";
		input.value = "";

		loadFiltered();
	},
	base = details({ "id": "filter" }, [
		summary("Filter"),
		div({ "class": "tabs" }, [
			details({ "name": "filter", "open": "open" }, [
				summary("Users"),
				div([
					userFilter,
					userSelect,
					br(),
					button({ "click": () => clearFilter(userFilter, userSelect) }, "Clear"),
					button({ "click": () => setFilter("users", userSelect) }, "Filter")
				])
			]),
			details({ "name": "filter" }, [
				summary("Groups"),
				div([
					groupFilter,
					groupSelect,
					br(),
					button({ "click": () => clearFilter(groupFilter, groupSelect) }, "Clear"),
					button({ "click": () => setFilter("groups", groupSelect) }, "Filter")
				])
			])
		])
	]);

export const filter = {
	"type": "none",
	"names": [] as string[]
};

let loadFiltered = () => { };

export default Object.assign(base, {
	"update": (path: string, _: DirectoryWithChildren, load: (path: string) => void) => {
		loadFiltered = () => load(path);
	},
	"init": (users: string[], groups: string[]) => {
		const userOpts: HTMLOptionElement[] = [],
			groupOpts: HTMLOptionElement[] = [];

		for (const user of users) {
			userOpts.push(option(user))
		}

		for (const group of groups) {
			groupOpts.push(option(group))
		}

		userSelect.append(...userOpts)
		groupSelect.append(...groupOpts)
	}
});