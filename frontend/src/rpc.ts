import type { ReportSummary, Tree } from './types.js';

const getURL = <T>(url: string, params: Record<string, unknown> = {}, body?: string) => {
	return new Promise<T>((successFn, errorFn) => {
		const urlParams: string[] = [];

		for (const [key, value] of Object.entries(params)) {
			urlParams.push(`${key}=${encodeURIComponent(value + "")}`);
		}

		if (urlParams.length) {
			url += "?" + urlParams.join("&");
		}

		const xh = new XMLHttpRequest();

		xh.open(body ? "POST" : "GET", url);
		xh.addEventListener("readystatechange", () => {
			if (xh.readyState === 4) {
				if (xh.status === 204 || !xh.responseText.length) {
					successFn(null as T);
				} else if (xh.status === 200) {
					successFn(JSON.parse(xh.responseText));
				} else {
					errorFn(new Error(xh.responseText));
				}
			}
		});
		xh.send(body);
	});
};

export const getTree = (dir: string) => getURL<Tree>("api/tree", { dir }),
	claimDir = (dir: string) => getURL<void>("api/dir/claim", { dir }),
	passDirClaim = (dir: string, passTo: string) => getURL<void>("api/dir/pass", { dir, passTo }),
	revokeDirClaim = (dir: string) => getURL<void>("api/dir/revoke", { dir }),
	createRule = (dir: string, action: string, match: string, metadata: string) => getURL<void>("api/rules/create", { dir, action, match, metadata }),
	updateRule = (dir: string, action: string, match: string, metadata: string) => getURL<void>("api/rules/update", { dir, action, match, metadata }),
	removeRule = (dir: string, match: string) => getURL<void>("api/rules/remove", { dir, match }),
	getReportSummary = () => getURL<ReportSummary>("api/report/summary"),
	uploadFOFN = (dir: string, action: string, metadata: string, files: string[]) => getURL<void>("api/uploadfofn",{ dir, action, metadata }, JSON.stringify(files)),
	setDirDetails = (dir: string, frequency: number, reviewDate: string, removeDate: string) => getURL<void>("api/dir/setdetails", { dir, frequency, reviewDate, removeDate }),
	user = await getURL<string>("api/whoami");