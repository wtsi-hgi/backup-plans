import { div, h2, p, h4, datalist, table, thead, tbody, th, td, tr } from "./lib/html.js";
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

// TODO: make filter actually work somehow

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
                        tr(td({ "class": "path", "colspan": "5" }, path)),
                        Array.from(rulestats).map((rule) => [
                            tr({}, [
                                td({ "class": "collapsible" }, rule.BackupType != null ? BackupType.from(rule.BackupType).toString() : "Unplanned"),
                                td({ "class": "collapsible" }, rule.Match),
                                td({ "class": "collapsible" }, "Size: " + formatBytes(rule.size)),
                                td({ "class": "collapsible" }, "Count: " + rule.count)
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
