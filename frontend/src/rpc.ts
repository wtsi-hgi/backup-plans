export type Stats = {
	ID: number;
	MTime: number;
	Files: number;
	Size: number;
}

export type Summary = {
	User: string;
	Group: string;
	Users: Record<string, Stats>;
	Groups: Record<string, Stats>;
}

export type Tree = Record<string, Summary>;

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
				} else {
					errorFn(new Error(xh.responseText));
				}
			}
		});
		xh.send();
	});
};

export const getTree = (dir: string) => getURL<Tree>("/api/tree", { dir });