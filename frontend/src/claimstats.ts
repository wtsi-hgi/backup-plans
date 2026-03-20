import { div, h2, p, button, table, thead, tbody, th, td, tr, fieldset, legend, input, datalist, option } from "./lib/html.js";
import { getClaimStats, user } from "./rpc.js";
import { formatBytes, longAgoStr, createSpinner } from "./lib/utils.js";
import { BackupType, ibackupStatusColumns } from "./consts.js";
import { load } from './load.js';
import { amendNode, clearNode } from "./lib/dom.js";
import { users, groups, bomSet } from './userGroups.js';
import type { SetBackupActivity } from "./types.js";
import { svg, use } from './lib/svg.js';

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

function dedupeSBA(sba: SetBackupActivity[]) {
    const seen = new Set();

    return sba.filter((sba) => {
        if (seen.has(sba.Name)) {
            return false;
        }

        seen.add(sba.Name);
        return true;
    })
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
                    ]),
                    table({ "class": "summary" }, [
                        thead(tr([
                            th("Backup Name"),
                            th("Last Backup"),
                            th("Status")
                        ])),
                        tbody(dirStats.BackupStatus.length > 0 ? dedupeSBA(dirStats.BackupStatus).map((sba) => [
                            tr([
                                td(sba.Name),
                                td(sba.LastSuccess === "0001-01-01T00:00:00Z" ? "Pending" : longAgoStr(sba.LastSuccess)),
                                td({
                                    "class": "tooltip",
                                    "data-tooltip": ibackupStatusColumns
                                        .filter(c => sba[c])
                                        .map(c => {
                                            const v = sba[c];

                                            if (c === "Failures" && v === -1) {
                                                return `${c}: N/A`;
                                            }

                                            return `${c}: ${sba[c].toLocaleString()}`
                                        })
                                        .join("\n") || false
                                }, sba.Failures === -1 ? "-"
                                    : sba.Failures === 0 ? svg(use({ "href": "#tickIcon" })) : svg(use({ "href": "#crossIcon" })))
                            ])
                        ]) : tr(td({ "colspan": "3" }, "No backup sets.")))
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
            "keydown": function (this: HTMLInputElement, e: KeyboardEvent) { if (e.key === "Enter") filterClaimStats() },
            "dblclick": function (this: HTMLInputElement) { this.select(); }
        }),
        input({
            "placeholder": "Group, BOM",
            "list": "claimstatsGroupBoms",
            "input": function (this: HTMLInputElement) { filter.groupbom = this.value },
            "keydown": function (this: HTMLInputElement, e: KeyboardEvent) { if (e.key === "Enter") filterClaimStats() },
            "dblclick": function (this: HTMLInputElement) { this.select(); }
        }),
        button({ "click": filterClaimStats }, "Filter"),
    ]);
}

initialiseClaimStats()
export default base
