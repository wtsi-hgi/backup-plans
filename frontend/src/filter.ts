import type { DirectoryWithChildren } from './types.js';
import { br, button, datalist, details, div, input, option, select, summary } from './lib/html.js';
import { setAndReturn, stringSort } from './lib/utils.js';
import { boms, owners, userGroups } from './userGroups.js';
import { handle, inputState, selectState, set, tab } from './state.js';

const userOpts: HTMLOptionElement[] = [],
	groupOpts: HTMLOptionElement[] = [],
	reverseOwners = new Map<string, string[]>(),
	reverseBoms = new Map<string, string[]>(),
	userSelect = selectState({ "id": "selectedUsers", "multiple": "multiple" }),
	groupSelect = selectState({ "id": "selectedGroups", "multiple": "multiple" }),
	userFilter = inputState({
		"id": "userFilter",
		"placeholder": "Filter: User", "list": "userList", "input": function (this: HTMLInputElement) {
			userSelect.replaceChildren(...userOpts.filter(opt => opt.innerText.includes(this.value)).map(e => (e.selected = e.value === this.value, e)));
		}
	}),
	groupFilter = inputState({
		"id": "groupFilter",
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
		filter["names"] = [];
		input.value = "";
		select.replaceChildren(...options.map(opt => (opt.selected = false, opt)));

		select.dispatchEvent(new CustomEvent("input"));
		input.dispatchEvent(new CustomEvent("input"));
		loadFiltered();
	},
	base = tab({ "id": "filter", "name": "", "summary": "Filter" }, [
		div({ "class": "tabs" }, [
			tab({ "name": "filterTab", "summary": "Users", "open": true }, [
				div([
					userFilter,
					userSelect,
					userList,
					br(),
					button({ "click": () => clearFilter(userFilter, userSelect, userOpts) }, "Clear"),
					button({ "click": () => setFilter("users", userSelect) }, "Filter")
				])
			]),
			tab({ "name": "filterTab", "summary": "Groups" }, [
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

let loadFiltered = () => { },
	debounce = false;

handle("filterType", v => {
	filter["type"] = v;

	if (!debounce) {
		debounce = true;

		queueMicrotask(loadFiltered);
	}
});

handle("filterNames", v => {
	filter["names"] = JSON.parse(v || "[]");

	if (!debounce) {
		debounce = true;

		queueMicrotask(loadFiltered);
	}
});

export default Object.assign(base, {
	"update": (path: string, _: DirectoryWithChildren, load: (path: string) => void) => {
		loadFiltered = () => {
			debounce = false;

			set("filterType", filter.type);
			set("filterNames", JSON.stringify(filter.names));
			load(path);
		}
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