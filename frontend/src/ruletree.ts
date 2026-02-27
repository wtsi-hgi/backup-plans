import type { DirectoryWithChildren, RuleStats } from './types.js';
import type { Children } from "./lib/dom.js";
import { clearNode } from "./lib/dom.js";
import { div, h2, span } from "./lib/html.js";
import { action, formatBytes, stringSort } from "./lib/utils.js";
import { load, registerLoader } from './load.js';

export type TreeNode = {
	children: Record<string, TreeNode>;
	rules: RuleStats[];
	fullPath?: string;
};

const base = div({ "id": "affectingRules" }),
	buildTree = (flatList: [string, RuleStats[]][]): Record<string, TreeNode> => {
		const root: Record<string, TreeNode> = {};

		for (const [fullPath, rules] of flatList) {
			const parts = fullPath.split(/(?<=\/)/).filter(Boolean);
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
	},
	hasDeepRules = (node: TreeNode): boolean => {
		if (node.rules.length > 0) {
			return true;
		}

		return Object.values(node.children).some(hasDeepRules);
	},
	getRuleSummaryElements = (rules: RuleStats[]): Children => {
		if (!rules || rules.length === 0) {
			return [];
		}

		return rules.map((rule) => {
			const actionText = action(rule.BackupType);

			return div({ "class": "rule-badge" }, [
				span({ "class": "file-type", "data-override": rule.Override }, rule.Match),
				span({ "class": "file-summary" }, ` ${rule.count.toLocaleString()} files • ${formatBytes(rule.size)} `),
				span({ "class": "action-pill", "data-action": actionText }, actionText),
			]);
		});
	},
	renderTree = (path: string, node: Record<string, TreeNode>, parentPath = "", depth = 0): Children => Object.entries(node).map(([dir, data]) => {
		const fullPath = parentPath + dir,
			hasChildren = Object.keys(data.children).length > 0,
			treeChildren = hasChildren ? div({ "class": "tree-children" }, renderTree(path, data.children, fullPath, depth + 1)) : [],
			ruleChildren: Children = [
				span({ "class": "dir-title" }, dir),
				getRuleSummaryElements(data.rules),
			];

		return div({ class: "rule-section" }, [
			div({
				"class": "summary-header",
				"click": (e: Event) => load(data.fullPath || fullPath).then(() => window.scrollTo(0, 0))
			}, ruleChildren),
			treeChildren
		]);
	});

registerLoader((path: string, data: DirectoryWithChildren) => {
	const rulesList = Object.entries(data.rules)
		.map(([path, rule]) => [path, rule.filter(r => r.count)] as [string, RuleStats[]])
		.filter(([dir, rules]) => dir && dir !== path && rules.length)
		.toSorted(([a], [b]) => stringSort(a, b));

	if (!Object.entries(rulesList).some(([dir, rules]) => dir && dir !== path && rules.length)) {
		clearNode(base);

		return;
	}

	clearNode(
		base,
		div({ "class": "card-container" }, [
			div({ "class": "card-header" }, h2({ "class": "card-title" }, "Rules affecting files within this directory tree")),
			renderTree(path, buildTree(rulesList)),
		])
	);
});

export default base;