import Breadcrumbs from './breadcrumbs.js';
import Load from './data.js';
import DiskTree from './disktree.js';
import DirInfo from './info.js';
import Rules from './rules.js';
import { symbols } from './symbols.js';

(document.readyState === "complete" ? Promise.resolve() : new Promise(successFn => window.addEventListener("load", successFn, { "once": true }))).then(() => {
	const load = (path: string) => Load(path).then(data => {
		Breadcrumbs.update(path, load);
		DiskTree.update(path, data, load);
		DirInfo.update(path, data, load);
		Rules.update(path, data, load);
	});

	return load("/");
})
	.then(() => {
		document.body.replaceChildren(
			symbols,
			Breadcrumbs,
			DiskTree,
			DirInfo,
			Rules
		);
	});