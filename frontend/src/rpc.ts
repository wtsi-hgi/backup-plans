import type { ReportSummary, Tree, UserGroups, DirStats } from './types.js';

const encodeForm = (params: Record<string, unknown>) => {
	const u = new URLSearchParams();

	for (const [k, v] of Object.entries(params)) {
		if (v instanceof Array) {
			for (const e of v) {
				u.append(k, e);
			}
		} else {
			u.set(k, v + "");
		}
	}

	return u;
},
	getURL = <T>(url: string, params: Record<string, string> = {}, body?: string | Record<string, unknown>) => {
		return new Promise<T>((successFn, errorFn) => {
			const urlParams = encodeForm(params).toString(),
				xh = new XMLHttpRequest();

			if (urlParams.length) {
				url += "?" + urlParams;
			}

			xh.open(body ? "POST" : "GET", url);
			xh.addEventListener("readystatechange", () => {
				if (xh.readyState === xh.DONE) {
					if (xh.status === 204 || !xh.responseText.length) {
						successFn(null as T);
					} else if (xh.status === 200) {
						successFn(JSON.parse(xh.responseText));
					} else {
						errorFn(new Error(xh.responseText));
					}
				}
			});
			xh.send(body ? typeof body === "string" ? body : encodeForm(body) : null);
		});
	};

export const getTree = (dir: string) => getURL<Tree>("api/tree", { dir }),
	claimDir = (dir: string) => getURL<void>("api/dir/claim", {}, { dir }),
	passDirClaim = (dir: string, passTo: string) => getURL<void>("api/dir/pass", {}, { dir, passTo }),
	revokeDirClaim = (dir: string) => getURL<void>("api/dir/revoke", {}, { dir }),
	createRule = (dir: string, action: string, match: string[], metadata: string, override: boolean) => getURL<void>("api/rules/create", {}, { dir, action, match, metadata, override }),
	updateRule = (dir: string, action: string, match: string, metadata: string) => getURL<void>("api/rules/update", {}, { dir, action, match, metadata }),
	removeRule = (dir: string, match: string) => getURL<void>("api/rules/remove", {}, { dir, match }),
	getReportSummary = () => getURL<ReportSummary>("api/report/summary"),
	setDirDetails = (dir: string, frequency: number, frozen: boolean, review: number, remove: number) => getURL<void>("api/dir/setdetails", {}, { dir, frequency, frozen, review, remove }),
	setExists = (dir: string, metadata: string) => getURL<boolean>("api/setExists", { dir, metadata }),
	getUserGroups = () => getURL<UserGroups>("api/usergroups"),
	getMainProgrammes = () => getURL<string[]>("api/mainprogrammes"),
	getClaimStats = (user: string, groupbom: string) => getURL<DirStats[]>("api/claimstats", {}, { user, groupbom }),
	user = await getURL<string>("api/whoami");
