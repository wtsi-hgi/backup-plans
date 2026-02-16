import { div, h2, p, h4, span, table, thead, tbody, th, td, tr } from "./lib/html.js";
import type { UserClaims } from "./types.js";
import { getClaimStats } from "./rpc.js";
import { inputState } from "./state.js";
import { formatBytes } from "./lib/utils.js";
import { BackupType } from "./consts.js";

const base = div();

function initialiseClaimStats() {
    console.log("Initialising claim stats page");
    getClaimStats().then(claimStats => {
        base.append(createClaimStatsPage(claimStats));
    });
}

// TODO: 
// 1) make name search actually work somehow
// 2) colour name bars by something (im thinking unplanned amount)

function toggleRules(this: HTMLTableRowElement) {
    const arrow = this.querySelector(".arrow") as HTMLElement;
    arrow.textContent = arrow.textContent === "▶" ? "▼" : "▶";

    let nextRow = this.nextElementSibling;
    while (nextRow) {
        if (nextRow.classList.contains("path-row")) break;

        nextRow.classList.toggle("hidden");
        nextRow = nextRow.nextElementSibling;
    }
}

function createClaimStatsPage(claimStats: UserClaims) {
    const filterUserClaims = inputState({ "id": "claimstatsFilter", "placeholder": "Name", "list": "userList" });

    return div({}, [
        div({ "class": "claimstats-container" }, [
            filterUserClaims,
        ]),
        Object.entries(claimStats).map(([user, dirClaims]) => div({ "class": "claimstats-container" }, [
            table({ "class": "userclaims" }, [
                thead(
                    tr(th({ "colspan": "5" }, user)),
                ),
                tbody({}, [
                    Object.entries(dirClaims).map(([path, rulestats]) => [
                        tr({ "class": "path-row", "click": toggleRules }, td({ "colspan": "5" }, [span({ class: "arrow" }, "▶"), path])),
                        Array.from(rulestats).map((rule) => [
                            tr({ "class": "rule-row, hidden" }, [
                                td(rule.BackupType != null ? BackupType.from(rule.BackupType).toString() : "Unplanned"),
                                td(rule.Match),
                                td("Size: " + formatBytes(rule.size)),
                                td("Count: " + rule.count)
                            ])
                        ])
                    ])
                ])
            ]),
        ]))
    ]);
}

initialiseClaimStats()
export default base
