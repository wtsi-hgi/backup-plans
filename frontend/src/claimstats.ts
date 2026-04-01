import { div, h2, p, button, table, thead, tbody, th, td, tr, fieldset, legend, input, datalist, option } from "./lib/html.js";
import { getClaimStats, user } from "./rpc.js";
import { formatBytes, longAgoStr, createSpinner } from "./lib/utils.js";
import { BackupType, ibackupStatusColumns } from "./consts.js";
import { load } from './load.js';
import { amendNode, clearNode } from "./lib/dom.js";
import { users, groups, bomSet } from './userGroups.js';
import type { SetBackupActivity, DirStats, RuleInfo } from "./types.js";
import { svg, use } from './lib/svg.js';
import { setState } from "./state.js";

const base = div({ "class": "main-container" });
const container = div();
const filter = {
    "user": "",
    "groupbom": ""
};

function initialiseClaimStats() {
    base.append(createFilterSection());

    let hasFilter = false;

    for (const [key, value] of new URLSearchParams(window.location.search)) {
        if (key === "csUser" || key === "csGroupBom") {
            hasFilter = true;
            filter[key === "csUser" ? "user" : "groupbom"] = value;
            userInput.value = filter.user;
            groupBomInput.value = filter.groupbom;
        }
    }

    if (!hasFilter) {
        setState("csUser", user);
        filter.user = user;
    }

    updateClaimStats();

    base.append(container);
}

export function updateClaimStats() {
    clearNode(container);
    amendNode(container, createClaimStatsSection());
}


type row = {
    matches: string[],
    size: bigint,
    count: bigint,
    backup: string,
    name: string,
    sba?: SetBackupActivity
};

function prepareData(dirStats: DirStats) {
    const rowMap: Map<string, Map<string, row>> = new Map(); // backuptype->backupname->row
    const backupMap = new Map(Object.values(dirStats.BackupStatus).map(sba => [sba.Name, sba]));
    const iBackupSba = Object.values(dirStats.BackupStatus).find(sba => sba.Name.startsWith("plan::"));

    if (iBackupSba !== undefined) {
        backupMap.set(iBackupSba!.Name, iBackupSba!)
    }

    for (const rule of dirStats.RuleStats) {
        if (rule.BackupType === undefined) {
            // unplanned data
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

    return rows
}

function makeRow(rowMap: Map<string, Map<string, row>>, backupMap: Map<string, SetBackupActivity>, rule: RuleInfo, backupName: string) {
    if (rule.BackupType === undefined) { return }

    const backuptype = BackupType.from(rule.BackupType);
    const label = backuptype.optionLabel();
    const btype =
        label.startsWith("Manual Backup: ") && label !== "Manual Backup: iBackup"
            ? label.slice("Manual Backup: ".length)
            : label;

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
    if (row.backup === "No Backup" || row.backup === "Unchecked" || row.backup === "Prefect") { return [td("-"), td("-")] }
    if (row.sba === undefined) { console.log("Undefined sba for row:", row); return [td("Unknown"), td("-")] }

    const sba = row.sba!;
    const tooltip = [
        `Last Modified: ${lastMod === 0 ? "None" : new Date(lastMod * 1000).toLocaleString()}`,
        `Last Backup: ${sba.LastSuccess === "0001-01-01T00:00:00Z" ? "None" : new Date(sba.LastSuccess).toLocaleString()}`
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
                    +new Date(lastMod * 1000) > +new Date(sba.LastSuccess) ? svg(use({ "href": "#crossIcon" })) : svg(use({ "href": "#tickIcon" }))
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

function buildTableRow(dirStats: DirStats, row: row) {
    const first = row.matches.slice(0, 10);
    const rest = row.matches.length > 10 ? row.matches.slice(10) : [];

    return tr({}, [
        td({
            "class": "tooltip status",
            "data-tooltip": (rest.join(", ") || false)
        }, rest.length > 0 ? first.join(", ") + "..." : first.join(", ")),
        td(formatBytes(row.size)),
        td(row.count.toLocaleString()),
        td(row.backup),
        td(row.name),
        getStatusTd(dirStats.LastMod, row)
    ])
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
                                buildTableRow(dirStats, row),
                            ]) : tr(td({ "colspan": "7" }, "No rules added to this directory."))
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
        setState("csUser", filter.user);
        setState("csGroupBom", filter.groupbom);
        updateClaimStats()
    } else {
        alert("Please enter a user and/or group to filter by.");
    }
}

const userInput = input({
    "placeholder": "Username",
    "list": "claimstatsUsers",
    "value": user,
    "input": function (this: HTMLInputElement) { filter.user = this.value },
    "keydown": function (this: HTMLInputElement, e: KeyboardEvent) { if (e.key === "Enter") filterClaimStats() },
    "dblclick": function (this: HTMLInputElement) { this.select(); }
});
const groupBomInput = input({
    "placeholder": "Group, BOM",
    "list": "claimstatsGroupBoms",
    "input": function (this: HTMLInputElement) { filter.groupbom = this.value },
    "keydown": function (this: HTMLInputElement, e: KeyboardEvent) { if (e.key === "Enter") filterClaimStats() },
    "dblclick": function (this: HTMLInputElement) { this.select(); }
});

function createFilterSection() {
    return div({ "class": "claimstats-filter-container" }, [
        userList,
        groupBomList,
        userInput,
        groupBomInput,
        button({ "click": filterClaimStats }, "Filter"),
    ]);
}

initialiseClaimStats()
export default base
