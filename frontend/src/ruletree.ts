import type { DirectoryWithChildren, RuleStats, TreeNode } from "./types.js";
import { clearNode } from "./lib/dom.js";
import { button, details, div, h2, input, span, summary } from "./lib/html.js";
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

    // Returns true if the node or any of its descendants has rules
    const hasDeepRules = (node: TreeNode): boolean => {
      if (node.__rules.length > 0) return true;
      return Object.values(node.__children).some(hasDeepRules);
    };

    const renderTree = (
      node: Record<string, TreeNode>,
      parentPath = "",
      depth = 0
    ): HTMLElement[] => {
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
            `\u007B${fileType}\u007D • ${count} files • ${size} • ${actionText}`
          );
        });
      };

      const toggleAllPaths = (
        node: TreeNode,
        detail: HTMLDetailsElement,
        open: boolean
      ) => {
        detail.open = open;

        const childDetails = Array.from(
          detail.querySelectorAll(":scope > .tree-children > details")
        ) as HTMLDetailsElement[];

        for (const [childName, childNode] of Object.entries(node.__children)) {
          if (!hasDeepRules(childNode)) continue;

          const childDetail = childDetails.find(
            (d) => d.querySelector(".dir-title")?.textContent === childName
          );

          if (childDetail) {
            toggleAllPaths(childNode, childDetail, open);
          }
        }
      };

      return Object.entries(node).map(([dir, data]) => {
        const fullPath = parentPath + "/" + dir;
        const hasRules = data.__rules.length > 0;
        const hasChildren = Object.keys(data.__children).length > 0;

        const detailEl = details({ class: "rule-section" });
        if (depth < 4) detailEl.open = true;

        let expandIndicator: HTMLElement | null = null;

        if (hasChildren) {
          expandIndicator = span({
            class: "expand-indicator",
          });
          expandIndicator.textContent = detailEl.open ? "–" : "+";

          expandIndicator.addEventListener("click", (e) => {
            e.stopPropagation();
            e.preventDefault();

            const isOpen = detailEl.open;

            toggleAllPaths(data, detailEl, !isOpen);
          });

          detailEl.addEventListener("toggle", () => {
            expandIndicator!.textContent = detailEl.open ? "–" : "+";
          });
        }

        const summaryChildren: (HTMLElement | SVGElement | null)[] = [
          expandIndicator,
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
            : null,
          ...getRuleSummaryElements(data.__rules),
        ];

        const summaryEl = summary(
          {
            class: "summary-header",
            click: (e: MouseEvent) => {
              e.preventDefault();
            },
          },
          summaryChildren.filter(
            (el): el is HTMLElement | SVGElement => el !== null
          )
        );

        detailEl.append(summaryEl);

        if (hasChildren) {
          const childDetails = renderTree(data.__children, fullPath, depth + 1);
          detailEl.append(div({ class: "tree-children" }, childDetails));
        }
        return detailEl;
      });
    };

    clearNode(base);
    const card = div({ class: "card-container" }, [
      div({ class: "card-header" }, [
        h2(
          { class: "card-title" },
          "Rules affecting files within this directory tree"
        ),
      ]),
      ...renderTree(buildTree(rulesList)),
    ]);
    base.append(card);
  },
});
