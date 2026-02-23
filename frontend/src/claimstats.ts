import { div, h2, p, button, table, thead, tbody, th, td, tr, fieldset, legend, input, datalist, option } from "./lib/html.js";
import { getClaimStats, user } from "./rpc.js";
import { formatBytes, longAgoStr } from "./lib/utils.js";
import { BackupType } from "./consts.js";
import { svg, title, use } from "./lib/svg.js";
import { load } from './load.js';
import { amendNode, clearNode } from "./lib/dom.js";
import { users, groups, boms } from './userGroups.js';

const base = div({ "class": "main-container" });
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

function createClaimStatsSection() {
    let page = div({ "class": "claimstats-container" });
    getClaimStats(filter.user, filter.group).then(claimstats => {
        claimstats.length > 0 ? claimstats.map((dirStats) => {
            if (!users.has(dirStats.ClaimedBy)) {
                users.add(dirStats.ClaimedBy);
                userList.append(option({ "label": "User: " + dirStats.ClaimedBy }, dirStats.ClaimedBy));
            };
            if (!groups.has(dirStats.Group) && dirStats.Group != "") {
                groups.add(dirStats.Group);
                groupList.append(option({ "label": "Group: " + dirStats.Group }, dirStats.Group));
            };

            console.log("Path:", dirStats.Path, "Group:", dirStats.Group, "BOM:", boms.get(dirStats.Group))

            page.appendChild(fieldset({ "class": "userclaims", "data-user": dirStats.ClaimedBy, "data-group": dirStats.Group }, [
                legend({ "class": "claimstats-legend" }, [h2(dirStats.Path), button({
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
            ]))
        }
        ) : page.appendChild(h2("No claimed directories."))
    });

    return page
}

const userList = datalist({ "id": "claimstatsUsers" });
userList.append(...Array.from(users).map((user) => option({ "label": "User: " + user }, user)));

const groupList = datalist({ "id": "claimstatsGroups" });
groupList.append(...Array.from(groups).map((group) => option({ "label": "Group: " + group }, group)));

function filterClaimStats() {
    if (filter.user != "" || filter.group != "") {
        updateClaimStats()
    } else {
        alert("Please enter a user and/or group to filter by.");
    }
}

function createFilterSection() {
    return div({ "class": "claimstats-filter-container" }, [
        userList,
        groupList,
        input({
            "placeholder": "Username",
            "list": "claimstatsUsers",
            "value": user,
            "input": function (this: HTMLInputElement) { filter.user = this.value },
            "keypress": function (this: HTMLInputElement, e: KeyboardEvent) { if (e.key === "Enter") filterClaimStats() }
        }),
        input({
            "placeholder": "Group",
            "list": "claimstatsGroups",
            "input": function (this: HTMLInputElement) { filter.group = this.value },
            "keypress": function (this: HTMLInputElement, e: KeyboardEvent) { if (e.key === "Enter") filterClaimStats() }
        }),
        button({ "click": filterClaimStats }, "Filter"),
    ]);
}

initialiseClaimStats()
export default base
