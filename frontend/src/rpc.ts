import type { DirectoryRules, Tree } from './types.js';

const getURL = <T>(url: string, params: Record<string, unknown> = {}) => {
	return new Promise<T>((successFn, errorFn) => {
		const urlParams: string[] = [];

		for (const [key, value] of Object.entries(params)) {
			urlParams.push(`${key}=${encodeURIComponent(value + "")}`);
		}

		if (urlParams.length) {
			url += "?" + urlParams.join("&");
		}

		const xh = new XMLHttpRequest();

		xh.open("GET", url);
		xh.addEventListener("readystatechange", () => {
			if (xh.readyState === 4) {
				if (xh.status === 200) {
					successFn(JSON.parse(xh.responseText));
				} else if (xh.status === 204) {
					successFn(null as T);
				} else {
					errorFn(new Error(xh.responseText));
				}
			}
		});
		xh.send();
	});
};

export const whoami = () => getURL<string>("/api/whoami"),
	getTree = (dir: string) => getURL<Tree>("/api/tree", { dir }),
	claimDir = (dir: string) => getURL<DirectoryRules>("/api/dir/claim", { dir }),
	passDirClaim = (dir: string) => getURL<DirectoryRules>("/api/dir/pass", { dir }),
	revokeDirClaim = (dir: string) => getURL<DirectoryRules>("/api/dir/revoke", { dir }),
	getRules = (dir: string) => getURL<DirectoryRules>("/api/rules/get", { dir }),
	createRule = (dir: string, action: string, match: string) => getURL<void>("/api/rules/create", { dir, action, match, "review": 1, "remove": 1, "frequency": 7 }),
	removeRule = (dir: string, match: string) => getURL<void>("/api/rules/remove", { dir, match }),
	user = "mw31"; //await whoami();