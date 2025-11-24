import type { DirectoryWithChildren, UserGroups } from './types.js';
import { br, button, datalist, details, div, input, option, select, summary } from './lib/html.js';
import { stringSort } from './lib/utils.js';

const userOpts: HTMLOptionElement[] = [],
	groupOpts: HTMLOptionElement[] = [],
	owners = new Map<string, string[]>(),
	boms = new Map<string, string[]>(),
	userSelect = select({ "multiple": "multiple" }),
	groupSelect = select({ "multiple": "multiple" }),
	userFilter = input({
		"placeholder": "Filter", "list": "userList", "input": function (this: HTMLInputElement) {
			userSelect.replaceChildren(...userOpts.filter(opt => opt.innerText.includes(this.value)));
		}
	}),
	groupFilter = input({
		"placeholder": "Filter", "list": "groupList", "input": function (this: HTMLInputElement) {
			const bom = boms.get(this.value),
				owner = owners.get(this.value);

			if (bom) {
				groupSelect.replaceChildren(...groupOpts.filter(opt => bom.includes(opt.innerText)));
			} else if (owner) {
				groupSelect.replaceChildren(...groupOpts.filter(opt => owner.includes(opt.innerText)));
			} else {
				groupSelect.replaceChildren(...groupOpts.filter(opt => opt.innerText.includes(this.value)));
			}
		}
	}),
	userList = datalist({ "id": "userList" }),
	groupList = datalist({ "id": "groupList" }),
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
					userList,
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
					groupList,
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
		const users = data.Users.filter(u => u.trim()),
			groups = data.Groups.filter(g => g.trim());

		users.sort(stringSort);
		groups.sort(stringSort);

		userOpts.push(...users.map(u => option(u)))
		groupOpts.push(...groups.map(g => option(g)));

		userSelect.append(...userOpts);
		userList.append(...users.map(u => option({ "data-filter": JSON.stringify([u]), "label": "User: " + u }, u)));
		groupSelect.append(...groupOpts);

		for (const [bom, groups] of Object.entries(data.BOM)) {
			boms.set(bom, groups);

			groupList.append(option({ "label": "BOM: " + bom }, bom));
		}

		for (const [owner, groups] of Object.entries(data.Owners)) {
			owners.set(owner, groups);

			groupList.append(option({ "label": "Owner: " + owner }, owner));
		}

		groupList.append(...groups.map(g => option({ "label": "Group: " + g }, g)));
	}
});