import type { DirectoryWithChildren, RuleStats } from './types.js';
import type { Children } from "./lib/dom.js";
import { clearNode } from "./lib/dom.js";
import { button, details, div, h2, span, summary } from "./lib/html.js";
import { svg, title, use } from "./lib/svg.js";
import { action, formatBytes, stringSort } from "./lib/utils.js";

export type TreeNode = {
	children: Record<string, TreeNode>;
	rules: RuleStats[];
	fullPath?: string;
};

const base = div({ class: "affectingRules" });

export default Object.assign(base, {
	update: (path: string, data: DirectoryWithChildren, load: (path: string) => Promise<DirectoryWithChildren>) => {
		const hasAnyRules = Object.entries(data.rules).some(
			([dir, rules]) => dir && dir !== path && rules.length
		);

		if (!hasAnyRules) {
			clearNode(base);

			return;
		}

		const rulesList = Object.entries(data.rules)
			.filter(([dir, rules]) => dir && dir !== path && rules.length)
			.toSorted(([a], [b]) => stringSort(a, b));

		const buildTree = (flatList: [string, RuleStats[]][]): Record<string, TreeNode> => {
			const root: Record<string, TreeNode> = {};
			for (const [fullPath, rules] of flatList) {
				const parts = fullPath.split("/").filter(Boolean);
				let node = root;

				parts.forEach((part, i) => {
					if (!node[part]) {
						node[part] = { children: {}, rules: [] };
					}

					if (i === parts.length - 1) {
						node[part].rules = rules;
						node[part].fullPath = fullPath;
					}

					node = node[part].children;
				});
			}

			return root;
		};

		const hasDeepRules = (node: TreeNode): boolean => {
			if (node.rules.length > 0) {
				return true;
			}

			return Object.values(node.children).some(hasDeepRules);
		};

		const getRuleSummaryElements = (rules: RuleStats[]): Children => {
			if (!rules || rules.length === 0) {
				return [];
			}

			return rules.map((rule) => {
				const fileType = rule.Match;
				const count = rule.count.toLocaleString();
				const size = formatBytes(rule.size);
				const actionText = action(rule.BackupType);

				return div({ "class": "rule-badge", "title": `${fileType}: ${count} files, ${size}, ${actionText}` }, [
					span({ "class": "file-type" }, `${fileType}`),
					span({ "class": "file-summary" }, ` ${count} files • ${size} `),
					span({ "class": `action-pill`, "data-action": actionText }, actionText),
				]);
			});
		};

		const renderTree = (node: Record<string, TreeNode>, parentPath = "", depth = 0): Children => Object.entries(node).map(([dir, data]) => {
			const fullPath = parentPath + "/" + dir;
			const hasRules = data.rules.length > 0;
			const hasChildren = Object.keys(data.children).length > 0;
			const treeChildren = hasChildren ? div({ "class": "tree-children" }, renderTree(data.children, fullPath, depth + 1)) : [];
			const ruleChildren: Children = [
				svg({ "class": "folder-button" }, [
					title("Folder"),
					use({ "href": "#folder" }),
				]),
				span({ "class": "dir-title" }, dir),
				hasRules ? button({
					"class": "load-button",
					"click": (e: Event) => {
						e.stopPropagation();
						load(data.fullPath || fullPath).then(() => window.scrollTo(0, 0));
					}
				}, svg([title("Go to"), use({ href: "#goto" })])) : [],
				getRuleSummaryElements(data.rules),
			];

			if (hasChildren) {
				return details(
					{ "class": "rule-section", [depth !== Math.max(4, Array.from(path).reduce((a, b) => (b === "/" ? a + 1 : a), 0)) ? "open" : "closed"]: "" }, [
					summary({ class: "summary-header" }, ruleChildren),
					treeChildren
				]);
			}

			return div({ class: "summary-header" }, [
				ruleChildren,
				treeChildren
			]);
		});

		clearNode(
			base,
			div({ "class": "card-container" }, [
				div({ "class": "card-header" }, h2({ "class": "card-title" }, "Rules affecting files within this directory tree")),
				renderTree(buildTree(rulesList)),
			])
		);
	},
});
