import type { DirectoryWithChildren, UserGroups } from './types.js';
import { br, button, details, div, input, option, select, summary } from './lib/html.js';

const userOpts: HTMLOptionElement[] = [],
	groupOpts: HTMLOptionElement[] = [],
	userSelect = select({ "multiple": "multiple" }),
	groupSelect = select({ "multiple": "multiple" }),
	buildFilter = (s: HTMLSelectElement, opts: HTMLOptionElement[]) => input({
		"placeholder": "Filter", "input": function (this: HTMLInputElement) {
			s.replaceChildren(...opts.filter(opt => opt.innerText.includes(this.value)))
		}
	}),
	userFilter = buildFilter(userSelect, userOpts),
	groupFilter = buildFilter(groupSelect, groupOpts),
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
	clearFilter = (input: HTMLInputElement, select: HTMLSelectElement, options: HTMLOptionElement[]) => {
		for (const opt of options) {
			opt.selected = false;
		}

		filter["type"] = "none";
		input.value = "";
		select.replaceChildren(...options);

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
					button({ "click": () => clearFilter(userFilter, userSelect, userOpts) }, "Clear"),
					button({ "click": () => setFilter("users", userSelect) }, "Filter")
				])
			]),
			details({ "name": "filter" }, [
				summary("Groups"),
				div([
					groupFilter,
					groupSelect,
					br(),
					button({ "click": () => clearFilter(groupFilter, groupSelect, groupOpts) }, "Clear"),
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
	"init": (data: UserGroups) => {
		for (const user of data.Users) {
			if (user.trim()) {
				userOpts.push(option(user))
			}
		}

		for (const group of data.Groups) {
			if (group.trim()) {
				groupOpts.push(option(group))
			}
		}

		userSelect.append(...userOpts)
		groupSelect.append(...groupOpts)
	}
});