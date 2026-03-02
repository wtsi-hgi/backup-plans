import { div, h2, p, button, table, thead, tbody, th, td, tr, fieldset, legend, input, datalist, option } from "./lib/html.js";
import { getClaimStats, user } from "./rpc.js";
import { formatBytes, longAgoStr, createSpinner } from "./lib/utils.js";
import { BackupType } from "./consts.js";
import { svg, title, use } from "./lib/svg.js";
import { load } from './load.js';
import { amendNode, clearNode } from "./lib/dom.js";
import { users, groups, bomSet } from './userGroups.js';

const base = div({ "class": "main-container" });
const container = div();

function initialiseClaimStats() {
    base.append(createFilterSection());
    updateClaimStats();
    base.append(container);
}

export function updateClaimStats() {
    clearNode(container);
    amendNode(container, createClaimStatsSection());
}

const filter = {
    "user": user,
    "groupbom": ""
}

function createClaimStatsSection() {
    const page = div({ "class": "claimstats-container" });
    const spinner = createSpinner();
    page.appendChild(spinner);

    getClaimStats(filter.user, filter.groupbom).then(claimstats => {
        page.replaceChildren(...claimstats.length > 0 ? claimstats.map((dirStats) => {
            if (!users.has(dirStats.ClaimedBy)) {
                users.add(dirStats.ClaimedBy);
                userList.append(option({ "label": "User: " + dirStats.ClaimedBy }, dirStats.ClaimedBy));
            };

            return fieldset({ "class": "userclaims", "data-user": dirStats.ClaimedBy, "data-group": dirStats.Group }, [
                legend({ "class": "claimstats-legend" }, [h2({
                    "click": () => load(dirStats.Path).then(() => {
                        window.scrollTo(0, 0);
                        document.getElementsByTagName("summary")[0].click();
                    }).catch((e: Error) => {
                        alert("Error: " + e.message);
                    })
                }, dirStats.Path),]),
                div([
                    div({ "class": "claiminfo" }, [
                        p("Last successful backup: " + (dirStats.BackupStatus.LastSuccess === "0001-01-01T00:00:00Z" ? "Pending" : longAgoStr(dirStats.BackupStatus.LastSuccess))),
                        p("Failures: " + dirStats.BackupStatus.Failures),
                        (filter.groupbom !== "" && filter.user === "") ? p("Claimed by: " + dirStats.ClaimedBy) : []
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
        }) : [h2("No claimed directories.")]);
    }).catch((e: Error) => {
        alert("Error: " + e.message);
    })

    return page
};

const userList = datalist({ "id": "claimstatsUsers" });
userList.append(...Array.from(users).map((user) => option({ "label": "User: " + user }, user)));

const groupBomList = datalist({ "id": "claimstatsGroupBoms" });
groupBomList.append(...Array.from(groups).map((group) => option({ "label": "Group: " + group }, group)));
groupBomList.append(...Array.from(bomSet).map((bom) => option({ "label": "BOM: " + bom }, bom)));

function filterClaimStats() {
    if (filter.user !== "" || filter.groupbom !== "") {
        updateClaimStats()
    } else {
        alert("Please enter a user and/or group to filter by.");
    }
}

function createFilterSection() {
    return div({ "class": "claimstats-filter-container" }, [
        userList,
        groupBomList,
        input({
            "placeholder": "Username",
            "list": "claimstatsUsers",
            "value": user,
            "input": function (this: HTMLInputElement) { filter.user = this.value },
            "keypress": function (this: HTMLInputElement, e: KeyboardEvent) { if (e.key === "Enter") filterClaimStats() },
            "dblclick": function (this: HTMLInputElement) { this.select(); }
        }),
        input({
            "placeholder": "Group, BOM",
            "list": "claimstatsGroupBoms",
            "input": function (this: HTMLInputElement) { filter.groupbom = this.value },
            "keypress": function (this: HTMLInputElement, e: KeyboardEvent) { if (e.key === "Enter") filterClaimStats() }
        }),
        button({ "click": filterClaimStats }, "Filter"),
    ]);
}

initialiseClaimStats()
export default base
