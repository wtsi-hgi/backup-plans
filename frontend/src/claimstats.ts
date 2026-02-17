import { div, h2, p, h4, span, table, thead, tbody, th, td, tr, fieldset, legend, h1 } from "./lib/html.js";
import type { DirStats } from "./types.js";
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
// 2) colour name bars by something (im thinking unplanned amount) ?

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

function createClaimStatsPage(claimStats: DirStats[]) {
    // const filterUserClaims = inputState({ "id": "claimstatsFilter", "placeholder": "Name", "list": "userList" });

    return div({}, [
        div({ "class": "claimstats-container" }, [
            claimStats.map((dirStats) => fieldset({ "class": "userclaims" }, [
                legend(h1(dirStats.Path)),
                div({}, [
                    p("Last successful backup: " + (dirStats.BackupStatus.LastSuccess === "" ? "Pending" : dirStats.BackupStatus.LastSuccess)),
                    p("Failures: " + dirStats.BackupStatus.Failures),
                    table({ "class": "summary" }, [
                        thead(tr({}, [
                            th("Match"),
                            th("Backup Type"),
                            th("Size"),
                            th("Count"),
                        ])),
                        tbody({}, [
                            dirStats.RuleStats.map((rule) => [
                                tr({ "class": "rule-row" }, [
                                    td(rule.Match),
                                    td(rule.BackupType != null ? BackupType.from(rule.BackupType).toString() : "Unplanned"),
                                    td(formatBytes(BigInt(rule.size))),
                                    td("" + rule.count)
                                ])
                            ])
                        ])
                    ])
                ])
            ]),
            )
        ])
    ])
}

initialiseClaimStats()
export default base
