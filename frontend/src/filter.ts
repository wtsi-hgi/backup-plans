import type { DirectoryWithChildren } from './types.js';
import { br, button, datalist, details, div, input, option, select, summary } from './lib/html.js';
import { setAndReturn, stringSort } from './lib/utils.js';
import { boms, owners, userGroups } from './userGroups.js';

const userOpts: HTMLOptionElement[] = [],
	groupOpts: HTMLOptionElement[] = [],
	reverseOwners = new Map<string, string[]>(),
	reverseBoms = new Map<string, string[]>(),
	userSelect = select({ "multiple": "multiple" }),
	groupSelect = select({ "multiple": "multiple" }),
	userFilter = input({
		"placeholder": "Filter: User", "list": "userList", "input": function (this: HTMLInputElement) {
			userSelect.replaceChildren(...userOpts.filter(opt => opt.innerText.includes(this.value)).map(e => (e.selected = e.value === this.value, e)));
		}
	}),
	groupFilter = input({
		"placeholder": "Filter: Group, BOM, Owner", "list": "groupList", "input": function (this: HTMLInputElement) {
			const bom = reverseBoms.get(this.value),
				owner = reverseOwners.get(this.value);

			if (bom) {
				groupSelect.replaceChildren(...groupOpts.filter(opt => bom.includes(opt.innerText)).map(e => (e.selected = true, e)));
			} else if (owner) {
				groupSelect.replaceChildren(...groupOpts.filter(opt => owner.includes(opt.innerText)).map(e => (e.selected = true, e)));
			} else {
				groupSelect.replaceChildren(...groupOpts.filter(opt => opt.innerText.includes(this.value)).map(e => (e.selected = e.value === this.value, e)));
			}
		}
	}),
	userList = datalist({ "id": "userList" }),
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
		filter["type"] = "none";
		input.value = "";
		select.replaceChildren(...options.map(opt => (opt.selected = false, opt)));

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
	"init": () => {
		const users = userGroups.Users.filter(u => u.trim()),
			groups = userGroups.Groups.filter(g => g.trim());

		users.sort(stringSort);
		groups.sort(stringSort);

		userOpts.push(...users.map(u => option(u)))
		groupOpts.push(...groups.map(g => option(g)));

		userSelect.append(...userOpts);
		userList.append(...users.map(u => option({ "data-filter": JSON.stringify([u]), "label": "User: " + u }, u)));
		groupSelect.append(...groupOpts);

		for (const [group, bom] of boms) {
			(reverseBoms.get(bom) ?? setAndReturn(reverseBoms, bom, [])).push(group);
		}

		for (const [group, owner] of owners) {
			(reverseOwners.get(owner) ?? setAndReturn(reverseOwners, owner, [])).push(group);
		}
	}
});