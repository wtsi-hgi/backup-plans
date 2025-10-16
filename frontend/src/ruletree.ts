import type { DirectoryWithChildren, RuleStats, TreeNode } from "./types.js";
import { clearNode } from "./lib/dom.js";
import {
  button,
  details,
  div,
  h2,
  input,
  span,
  summary,
  table,
  tbody,
  td,
  th,
  thead,
  tr,
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
    let searchQuery = "";

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

    const container = div({ class: "rules-container" });
    const card = div({ class: "card-container" });

    const header = div({ class: "card-header" }, [
      h2(
        { class: "card-title" },
        "Rules affecting files within this directory tree"
      ),
      input({
        class: "search-input",
        placeholder: "🔎 Search directories",
        value: searchQuery,
        input: (e: any) => {
          searchQuery = e.target.value;
          renderTreeView();
        },
      }),
    ]);

    const renderTree = (
      node: Record<string, TreeNode>,
      parentPath = "",
      depth = 0
    ): HTMLElement[] => {
      return Object.entries(node).map(([dir, data]) => {
        const fullPath = parentPath + "/" + dir;
        const hasRules = data.__rules.length > 0;
        const hasChildren = Object.keys(data.__children).length > 0;

        const detailEl = details({ class: "rule-section" });
        if (depth < 1) detailEl.open = true;

        const children: HTMLElement[] = [];
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

        const summaryChildren: (HTMLElement | SVGElement)[] = [
          svg({ class: "folder-button" }, [
            title("Folder"),
            use({ href: "#folder" }),
          ]),
          span({ class: "dir-title" }, dir),
          ...getRuleSummaryElements(data.__rules),
        ];

        const summaryEl = summary({ class: "summary-header" }, summaryChildren);

        detailEl.append(summaryEl);

        if (hasRules) {
          const goBtn = button(
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
          );
          summaryEl.append(goBtn);
        }

        // if (hasRules) {
        //   children.push(
        //     table({ class: "summary" }, [
        //       thead(tr([th("Match"), th("Action"), th("Files"), th("Size")])),
        //       tbody(
        //         data.__rules.map((rule) =>
        //           tr([
        //             td(rule.Match),
        //             td(action(rule.BackupType)),
        //             td(rule.count.toLocaleString()),
        //             td(
        //               { title: rule.size.toLocaleString() },
        //               formatBytes(rule.size)
        //             ),
        //           ])
        //         )
        //       ),
        //     ])
        //   );
        // }

        if (hasChildren) {
          const childDetails = renderTree(data.__children, fullPath, depth + 1);
          children.push(div({ class: "tree-children" }, childDetails));
        }

        detailEl.append(...children);
        return detailEl;
      });
    };

    const renderTreeView = () => {
      clearNode(container);

      const filteredList = rulesList.filter(([dir]) =>
        dir.toLowerCase().includes(searchQuery.toLowerCase())
      );
      const treeData = buildTree(filteredList);
      container.append(...renderTree(treeData));
    };

    clearNode(base);
    card.append(header, container);
    base.append(card);
    renderTreeView();
  },
});
