import { div, h2, p, button, table, thead, tbody, th, td, tr, fieldset, legend, h1, input, select } from "./lib/html.js";
import type { DirStats } from "./types.js";
import { getClaimStats, user } from "./rpc.js";
import { formatBytes, longAgoStr } from "./lib/utils.js";
import { BackupType } from "./consts.js";
import { svg, title, use } from "./lib/svg.js";
import { load } from './load.js';
import { userList } from './filter.js';
import { groupList } from './report.js'
import { amendNode, clearNode } from "./lib/dom.js";

const base = div();
const container = div();

function initialiseClaimStats() {
    base.appendChild(createFilterSection());
    updateClaimStats();
    base.appendChild(container);
}

export function updateClaimStats() {
    clearNode(container);
    amendNode(container, createClaimStatsSection());
}

let filter = {
    "user": user,
    "group": ""
}

// function filterClaimStats(children: HTMLFieldSetElement[]) {
//     console.log(filter);
//     const filterUser = filter.user == "" ? false : true;
//     const filterGroup = filter.group == "" ? false : true;

//     for (const child of children) {
//         const user = child.dataset.user;
//         const group = child.dataset.group;

//         if ((!filterUser || user == filter.user) && (!filterGroup || group == filter.group)) {
//             child.classList.remove("hidden");
//         } else {
//             child.classList.add("hidden");
//         }
//     }
// };

function createClaimStatsSection() {
    let page = div();
    getClaimStats(filter.user, filter.group).then(claimstats => {
        console.log("getClaimStats returned", claimstats);
        claimstats.map((dirStats) =>
            page.appendChild(fieldset({ "class": "userclaims", "data-user": dirStats.ClaimedBy, "data-group": dirStats.Group }, [
                legend({ "class": "claimstats-legend" }, [h1(dirStats.Path), button({
                    "class": "load-button",
                    "click": () => load(dirStats.Path).then(() => {
                        window.scrollTo(0, 0);
                        document.getElementsByTagName("summary")[0].click();
                    })
                }, svg([title("Go to"), use({ href: "#goto" })]))]),
                div([
                    div({ "class": "claiminfo" }, [
                        p("Last successful backup: " + (dirStats.BackupStatus.LastSuccess === "0001-01-01T00:00:00Z" ? "Pending" : longAgoStr(dirStats.BackupStatus.LastSuccess))),
                        p("Failures: " + dirStats.BackupStatus.Failures),
                    ]),
                    table({ "class": "summary" }, [
                        thead(tr([
                            th("Match"),
                            th("Backup Type"),
                            th("Size"),
                            th("Count"),
                        ])),
                        tbody([
                            dirStats.RuleStats.length > 0 ? dirStats.RuleStats.map((rule) => [
                                tr([
                                    td(rule.BackupType != null ? rule.Match : "-"),
                                    td(rule.BackupType != null ? BackupType.from(rule.BackupType).optionLabel() : "Unplanned"),
                                    td(formatBytes(BigInt(rule.size))),
                                    td("" + rule.count)
                                ])
                            ]) : tr(td({ "colspan": "4" }, "No rules or unplanned data."))
                        ])
                    ])
                ])
            ])
            ))
    });

    return page
}

function createFilterSection() {
    return div({ "class": "claimstats-filter-container" }, [
        input({
            "placeholder": "Username", "list": "userList", "value": user, "input": function (this: HTMLInputElement) {
                // filter.user = ""
                // if (Array.from(userList.options).some(opt => opt.value === this.value)) {
                filter.user = this.value;
                // }
            }
        }),
        input({
            "placeholder": "Group", "list": "groupList", "input": function (this: HTMLInputElement) {
                // filter.group = "";
                // if (Array.from(groupList.options).some(opt => opt.value === this.value)) {
                filter.group = this.value;
                // }
            }
        }),
        button({
            "click": function () {
                if (filter.user != "" || filter.group != "") {
                    console.log("Calling updateClaimStats with user:", filter.user, "group:", filter.group);
                    updateClaimStats()
                } else {
                    console.log("Both filter and user are blank, not calling claimstats endpoint");
                }

            }
        }, "Submit"),
    ]);
}

initialiseClaimStats()
export default base
