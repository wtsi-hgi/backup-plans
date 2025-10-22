import type {Children} from "./lib/dom.js"
import type { DirectoryWithChildren, RuleStats, TreeNode } from "./types.js";
import { amendNode, clearNode } from "./lib/dom.js";
import {
  button,
  details,
  div,
  h2,
  span,
  summary,
} from "./lib/html.js";
import { svg, title, use } from "./lib/svg.js";
import { action, formatBytes, stringSort } from "./lib/utils.js";

const base = div({ class: "affectingRules" });

export default Object.assign(base, {
  update: (
    path: string,
    data: DirectoryWithChildren,
    load: (path: string) => Promise<DirectoryWithChildren>
  ) => {
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

    const buildTree = (
      flatList: [string, RuleStats[]][]
    ): Record<string, TreeNode> => {
      const root: Record<string, TreeNode> = {};
      for (const [fullPath, rules] of flatList) {
        const parts = fullPath.split("/").filter(Boolean);
        let node = root;
        parts.forEach((part, i) => {
          if (!node[part]) node[part] = { __children: {}, __rules: [] };
          if (i === parts.length - 1) {
            node[part].__rules = rules;
            node[part].__fullPath = fullPath;
          }
          node = node[part].__children as Record<string, TreeNode>;
        });
      }
      return root;
    };

    const hasDeepRules = (node: TreeNode): boolean => {
      if (node.__rules.length > 0) return true;
      return Object.values(node.__children).some(hasDeepRules);
    };

    const renderTree = (
      node: Record<string, TreeNode>,
      parentPath = "",
      depth = 0
    ): Children => {
      const getRuleSummaryElements = (rules: RuleStats[]): HTMLElement[] => {
        if (!rules || rules.length === 0) return [];

        return rules.map((rule) => {
          const fileType = rule.Match;
          const count = rule.count.toLocaleString();
          const size = formatBytes(rule.size);
          const actionText = action(rule.BackupType);

          return span(
            {
              class: "rule-badge",
              title: `${fileType}: ${count} files, ${size}, ${actionText}`,
            },
            `{${fileType}} • ${count} files • ${size} • ${actionText}`
          );
        });
      };

      return Object.entries(node).map(([dir, data]) => {
        const fullPath = parentPath + "/" + dir;
        const hasRules = data.__rules.length > 0;
        const hasChildren = Object.keys(data.__children).length > 0;
        const children = hasChildren ? div({ class: "tree-children" }, renderTree(data.__children, fullPath, depth + 1)) : [];

        const detailEl = (hasChildren ? summary : div)({class: "summary-header"}, [
          svg({ class: "folder-button" }, [
            title("Folder"),
            use({ href: "#folder" }),
          ]),
          span({ class: "dir-title" }, dir),
          hasRules
            ? button(
                {
                  class: "load-button",
                  click: (e: Event) => {
                    e.stopPropagation();
                    load(data.__fullPath || fullPath).then(() =>
                      window.scrollTo(0, 0)
                    );
                  },
                },
                svg([title("Go to"), use({ href: "#goto" })])
              )
            : [],
            getRuleSummaryElements(data.__rules),
          ]);

        if (hasChildren) {
          return amendNode(details({ class: "rule-section", [depth !== Math.max(
            4,
            Array.from(path).reduce((a, b) => (b === "/" ? a + 1 : a), 0)
          ) ? "open" : "closed"]: ""
        }, detailEl), children);
        }

        return amendNode(detailEl, children);
      });
    };

    clearNode(base, div({ class: "card-container" }, [
      div({ class: "card-header" }, [
        h2(
          { class: "card-title" },
          "Rules affecting files within this directory tree"
        ),
      ]),
      renderTree(buildTree(rulesList))
    ]));
  },
});
