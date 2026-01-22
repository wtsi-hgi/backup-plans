import { div } from './lib/html.js';
import Breadcrumbs from './breadcrumbs.js';
import DiskTree from './disktree.js';
import Filter from './filter.js';
import List from './list.js';
import Report from './report.js';
import Rules from './rules.js';
import RuleTree from './ruletree.js';
import Summary from './summary.js';
import UserStats from './userstats.js';
import { symbols } from './symbols.js';
import { tab } from './state.js';
import Graph from './graph.js';

// const load = (path: string) => Load(path).then(data => {
// 	Breadcrumbs.update(path, load);
// 	DiskTree.update(path, data, load);
// 	List.update(path, data, load);
// 	Filter.update(path, data, load);
// 	Summary.update(path, data, load);
// 	Rules.update(path, data, load);
// 	RuleTree.update(path, data, load);
// 	UserStats.update(path, data, load);

// 	return data;
// });

(document.readyState === "complete" ? Promise.resolve() : new Promise(successFn => window.addEventListener("load", successFn, { "once": true })))
	.then(() => document.body.replaceChildren(
		symbols,
		div({ "class": "tabs" }, [
			tab({ "name": "tabs", "summary": "Rule Tree", "open": true }, [
				Breadcrumbs,
				div({ "class": "tabs" }, [
					Filter,
					tab({ "name": "dirtabs", "summary": "Directory Tree", "open": true }, DiskTree),
					tab({ "id": "dirlistTab", "name": "dirtabs", "summary": "Directory List" }, List),
					UserStats
				]),
				Summary,
				Rules,
				RuleTree
			]),
			tab({ "name": "tabs", "summary": "Top Level Report" }, Report),
			tab({ "name": "tabs", "summary": "Graphical Report" }, Graph)
		])
	));
