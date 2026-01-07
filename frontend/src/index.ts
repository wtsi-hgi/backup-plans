import { details, div, summary } from './lib/html.js';
import Breadcrumbs from './breadcrumbs.js';
import Load from './data.js';
import DiskTree from './disktree.js';
import Filter from './filter.js';
import List from './list.js';
import Report from './report.js';
import Rules from './rules.js';
import RuleTree from './ruletree.js';
import Summary from './summary.js';
import UserStats from './userstats.js';
import { symbols } from './symbols.js';
import { getUserGroups, userGroups } from './rpc.js';

const load = (path: string) => Load(path).then(data => {
	Breadcrumbs.update(path, load);
	DiskTree.update(path, data, load);
	List.update(path, data, load);
	Filter.update(path, data, load);
	Summary.update(path, data, load);
	Rules.update(path, data, load);
	RuleTree.update(path, data, load);
	UserStats.update(path, data, load);

	return data;
});

(document.readyState === "complete" ? Promise.resolve() : new Promise(successFn => window.addEventListener("load", successFn, { "once": true })))
	.then(() => {
		Filter.init();
		Report.init(load);
	})
	.then(() => load("/"))
	.then(() => {
		document.body.replaceChildren(
			symbols,
			div({ "class": "tabs" }, [
				details({ "name": "tabs", "open": "open" }, [
					summary("Rule Tree"),
					Breadcrumbs,
					div({ "class": "tabs" }, [
						Filter,
						details({ "name": "dirtabs", "open": "open" }, [
							summary("Directory Tree"),
							DiskTree
						]),
						details({ "id": "dirlistTab", "name": "dirtabs" }, [
							summary("Directory List"),
							List
						]),
						UserStats
					]),
					Summary,
					Rules,
					RuleTree
				]),
				details({ "name": "tabs" }, [
					summary("Top Level Report"),
					Report
				])
			])
		);

		if (window.location.hash !== "") {
			window.location.hash = window.location.hash;
		}
	});