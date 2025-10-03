import { details, div, summary } from './lib/html.js';
import Breadcrumbs from './breadcrumbs.js';
import Load from './data.js';
import DiskTree from './disktree.js';
import DirInfo from './info.js';
import Filter from './filter.js';
import List from './list.js';
import Rules from './rules.js';
import Summary from './summary.js';
import Report from './report.js';
import { symbols } from './symbols.js';

const load = (path: string) => Load(path).then(data => {
	Breadcrumbs.update(path, load);
	DiskTree.update(path, data, load);
	List.update(path, data, load);
	DirInfo.update(path, data, load);
	Filter.update(path, data, load);
	Summary.update(path, data);
	Rules.update(path, data, load);

	return data;
});

(document.readyState === "complete" ? Promise.resolve() : new Promise(successFn => window.addEventListener("load", successFn, { "once": true })))
	.then(() => load("/"))
	.then((data) => Filter.init(data.users, data.groups))
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
						details({ "name": "dirtabs" }, [
							summary("Directory List"),
							List
						])
					]),
					DirInfo,
					Summary,
					Rules
				]),
				details({ "name": "tabs" }, [
					summary("Top Level Report"),
					Report
				])
			])
		);
	});