import { div, h2, p, button, table, thead, tbody, th, td, tr, fieldset, legend, input, datalist, option } from "./lib/html.js";
import { createRule, getClaimStats, user } from "./rpc.js";
import { formatBytes, longAgoStr, createSpinner } from "./lib/utils.js";
import { BackupType, ibackupStatusColumns } from "./consts.js";
import { load } from './load.js';
import { amendNode, clearNode } from "./lib/dom.js";
import { users, groups, bomSet } from './userGroups.js';
import type { SetBackupActivity, DirStats, RuleInfo } from "./types.js";
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


function prepareData(dirStats: DirStats) {
    // match[], Size (Total), Count(Total), Backup (BackupType:BackupName), LastBackup, Status

    type row = {
        matches: string[],
        size: bigint,
        count: bigint,
        backup: string,
        sba?: SetBackupActivity
    };

    const rows: row[] = [];
    const backupMap = new Map(Object.values(dirStats.BackupStatus).map(sba => [sba.Name, sba]));
    const iBackupSba = Object.values(dirStats.BackupStatus).find(sba => sba.Name.startsWith("plan::"));
    const iBackupRow: row = {
        matches: [],
        size: 0n,
        count: 0n,
        backup: "",
        sba: iBackupSba
    };

    for (const rule of dirStats.RuleStats) {
        console.log("Processing rule", rule);
        if (rule.BackupType === undefined) {
            // console.log("Undefined rule (unplanned??)", rule);
            continue;
        }

        if (+rule.BackupType === +BackupType.BackupIBackup) {
            // console.log("ibackup rule found");
            iBackupRow.matches.push(rule.Match)
            iBackupRow.size += BigInt(rule.size);
            iBackupRow.count += BigInt(rule.count);
            for (const sba of dirStats.BackupStatus) {
                if (sba.Name.startsWith("plan::")) {
                    iBackupRow.backup = "iBackup:" + sba.Name
                    iBackupRow.sba = sba
                    break;
                }
            }

            continue;
        }

        rows.push({
            matches: [rule.Match],
            size: rule.size,
            count: rule.count,
            backup: `${BackupType.from(rule.BackupType).toString()}:${rule.Metadata}`, // make only do :rule.metadata if manual
            sba: backupMap.get(rule.Metadata),
        });

    }

    // console.log(iBackupRow);

    if (iBackupRow.matches.length > 0) {
        rows.push(iBackupRow);
    }

    // console.log("backupMap:", backupMap);
    console.log("rows:", rows);

    return rows
}

function getStatusTd(lastMod: string, sba: SetBackupActivity) {
    return [
        sba.LastSuccess === "0001-01-01T00:00:00Z" ?
            sba.Failures === -1 ? [
                td("None"),
                td({ "class": "status" }, svg(use({ "href": "#crossIcon" })))
            ] : [
                td("Pending"),
                td("-")
            ] : [
                td(longAgoStr(sba.LastSuccess)),
                sba.Failures === -1 ? td({ "class": "status" },
                    +new Date(lastMod) > +new Date(sba.LastSuccess) ? svg(use({ "href": "#crossIcon" })) : svg(use({ "href": "#tickIcon" }))
                ) : td({
                    "class": "tooltip status",
                    "data-tooltip": ibackupStatusColumns // TODO: Add last mod to tooltip
                        .filter(c => sba[c])
                        .map(c => `${c}: ${sba[c].toLocaleString()}`)
                        .join("\n") || false
                }, sba.Failures > 0 ? svg(use({ "href": "#crossIcon" })) : svg(use({ "href": "#tickIcon" }))
                )
            ]
    ]
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
                            th("Size"),
                            th("Count"),
                            th("Backup"), // type: backupname
                            th("Last Backup"),
                            th("Status")
                        ])),
                        tbody([
                            dirStats.RuleStats.length > 0 ? prepareData(dirStats).map((row) => [
                                tr({}, [
                                    td(row.matches.join(", ")),
                                    td(formatBytes(row.size)),
                                    td(formatBytes(row.count)),
                                    td(row.backup),
                                    row.sba === undefined ? [td("Pending"), td("-")] : getStatusTd(dirStats.LastMod, row.sba!)
                                ])
                            ]) : tr(td({ "colspan": "6" }, "No rules or unplanned data."))
                            // dirStats.RuleStats.length > 0 ? dirStats.RuleStats.map((rule) => [
                            //     tr([
                            //         td(rule.BackupType != null ? BackupType.from(rule.BackupType).optionLabel() : "Unplanned"),
                            //         td(rule.BackupType != null ? rule.Match : "-"),
                            //         td(formatBytes(BigInt(rule.size))),
                            //         td("" + rule.count)
                            //     ])
                            // ]) : tr(td({ "colspan": "6" }, "No rules or unplanned data."))
                        ])
                    ]),
                    // table({ "class": "summary" }, [
                    //     thead(tr([
                    //         th("Backup Name"),
                    //         th("Last Backup"),
                    //         th("Status")
                    //     ])),
                    //     tbody(dirStats.BackupStatus.length > 0 ? dirStats.BackupStatus.map(sba => [
                    //         tr([
                    //             td(sba.Name),
                    //             getStatusTd(dirStats.LastMod, sba),
                    //         ])
                    //     ]) : tr(td({ "colspan": "3" }, "No backup sets.")))
                    // ])
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
