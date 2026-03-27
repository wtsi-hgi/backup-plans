import { div, h2, p, button, table, thead, tbody, th, td, tr, fieldset, legend, input, datalist, option, map, b, i } from "./lib/html.js";
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

type row = {
    matches: string[],
    size: bigint,
    count: bigint,
    backup: string,
    name: string,
    sba?: SetBackupActivity
};

// TODO: Add unplanned stats somehow?

// match[], Size (Total), Count(Total), Backup (BackupType:BackupName), LastBackup, Status
function prepareData(dirStats: DirStats) {
    console.log("Prepare data:", dirStats);

    const rowMap: Map<string, Map<string, row>> = new Map(); // backuptype->backupname->row
    const backupMap = new Map(Object.values(dirStats.BackupStatus).map(sba => [sba.Name, sba]));
    const iBackupSba = Object.values(dirStats.BackupStatus).find(sba => sba.Name.startsWith("plan::"));
    if (iBackupSba !== undefined) {
        backupMap.set(iBackupSba!.Name, iBackupSba!)
    }

    for (const rule of dirStats.RuleStats) {
        if (rule.BackupType === undefined) {
            // unplanned i assume
            continue;
        }

        const backuptype = BackupType.from(rule.BackupType);
        let backupName = "-"

        if (BackupType.manual.includes(backuptype)) {
            backupName = rule.Metadata
        } else if (backuptype === BackupType.BackupIBackup) {
            backupName = iBackupSba!.Name
        }

        makeRow(rowMap, backupMap, rule, backupName)
    }

    const rows: row[] = [];

    for (const map of rowMap.values()) {
        for (const row of map.values()) {
            rows.push(row)
        }
    }

    console.log("rows:", rows);

    return rows
}

function makeRow(rowMap: Map<string, Map<string, row>>, backupMap: Map<string, SetBackupActivity>, rule: RuleInfo, backupName: string) {
    if (rule.BackupType === undefined) {
        console.log("Undefined backup type on rule: ", rule);

        return
    }

    const backuptype = BackupType.from(rule.BackupType);
    const btype = backuptype.optionLabel()

    if (!rowMap.has(btype)) {
        rowMap.set(btype, new Map());
    }

    const rMap = rowMap.get(btype)

    if (!rMap!.has(backupName)) {
        rMap?.set(backupName, {
            matches: [rule.Match],
            size: BigInt(rule.size),
            count: BigInt(rule.count),
            backup: btype,
            name: backupName,
            sba: backupMap.get(backupName)!,
        })

        return
    }

    const row = rMap!.get(backupName)!
    row.matches.push(rule.Match);
    row.size += BigInt(rule.size);
    row.count += BigInt(rule.count);
}

// getStatusTd returns the table data cells for Last Backup and Status
//
// For manual types, if modifications to the matched files has occurred more recently
// than they were backed up (based on nfs mod time/commit date), the status will be X.
// For automatic backups, the status will be based on failure count (anything other than
// 0 will be X).
function getStatusTd(lastMod: number, row: row) {
    if (row.backup === "nobackup" || row.backup === "manualunchecked" || row.backup === "manualprefect") { return [td("-"), td("-")] }
    const sba = row.sba!;
    const tooltip = [
        `Last Modified: ${new Date(lastMod * 1000).toLocaleString()}`,
        `Last Backup: ${new Date(sba.LastSuccess).toLocaleString()}`
    ];

    return [
        sba.LastSuccess === "0001-01-01T00:00:00Z" ?
            sba.Failures === -1 ? [
                td("None"),
                td({ "class": "tooltip status", "data-tooltip": tooltip.join("\n") || false }, svg(use({ "href": "#crossIcon" })))
            ] : [
                td("Pending"),
                td("-")
            ] : [
                td(longAgoStr(sba.LastSuccess)),
                sba.Failures === -1 ? td({ "class": "tooltip status", "data-tooltip": tooltip.join("\n") || false },
                    +new Date(lastMod) > +new Date(sba.LastSuccess) ? svg(use({ "href": "#crossIcon" })) : svg(use({ "href": "#tickIcon" }))
                ) : td({
                    "class": "tooltip status",
                    "data-tooltip": (
                        tooltip.concat(ibackupStatusColumns
                            .filter(c => sba[c])
                            .map(c => `${c}: ${sba[c].toLocaleString()}`))
                            .join("\n") || false
                    )
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
                            th("Backup Type"),
                            th("Backup Name"),
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
                                    td(row.name),
                                    row.sba === undefined ? [td("Pending"), td("-")] : getStatusTd(dirStats.LastMod, row)
                                ])
                            ]) : tr(td({ "colspan": "7" }, "No rules or unplanned data."))
                        ])
                    ]),
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
