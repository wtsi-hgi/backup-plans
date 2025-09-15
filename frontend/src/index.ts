import { base } from './disktree.js';
import { symbols } from './symbols.js';

(document.readyState === "complete" ? Promise.resolve() : new Promise(successFn => window.addEventListener("load", successFn, { "once": true }))).then(() => {
	document.body.replaceChildren(
		symbols,
		base
	);
});