import { filter } from './filter.js';
import { clearNode } from './lib/dom.js';
import { table, tbody, td, th, thead, tr, details, summary } from './lib/html.js';
import { formatBytes, setAndReturn } from './lib/utils.js';
import type { ChildDirectory, DirectoryWithChildren, SizeCountStats } from './types.js';

const base = tbody();

const container = details({
    "id": "userStats",
}, [
    summary("User stats"),
    table({ "class": "summary" }, [
        thead(tr([
            th("User"),
            th("Total file Size"),
            th("Total file Count"),
            th("Unplanned Size"),
            th("Unplanned Count")
        ])),
        base
    ])
]);

let gdata: DirectoryWithChildren;

export const onHover = (child: string) => {
    if (child === "") {
        updateChild(gdata);

        return;
    }

    updateChild(gdata.children[child]);
}

const updateChild = (data: ChildDirectory) => {
    const userStats = calculateUserStats(data);

    clearNode(base, userStats.size > 0 ? Array.from(userStats).sort().map(([user, sizeCountStats]) => {
        return tr([
            td(user),
            td({ "title": sizeCountStats.size.toLocaleString() }, formatBytes(sizeCountStats.size)),
            td(sizeCountStats.count.toLocaleString()),
            td({ "title": sizeCountStats.unplannedSize.toLocaleString() }, formatBytes(sizeCountStats.unplannedSize)),
            td(sizeCountStats.unplannedCount.toLocaleString())
        ])
    }) : tr(td({ "colspan": "5" }, "No files")));
},
    update = (_: string, data: DirectoryWithChildren, _load: (path: string) => void) => {
        gdata = data;

        updateChild(data);
    };

export default Object.assign(container, { update });

function calculateUserStats(dir: ChildDirectory) {
    const userStats = new Map<string, SizeCountStats>();

    for (const rule of dir.ruleSummaries) {
        for (const user of rule.Users) {
            if (filter["type"] === "users" && !filter["names"].includes(user.Name)) {
                continue;
            }

            const ustats = userStats.get(user.Name) ?? setAndReturn(userStats, user.Name, { size: 0n, count: 0n, unplannedSize: 0n, unplannedCount: 0n });

            if (rule.ID === 0) {
                ustats.unplannedSize += BigInt(user.Size);
                ustats.unplannedCount += BigInt(user.Files);
            }

            ustats.size += BigInt(user.Size);
            ustats.count += BigInt(user.Files);
        }
    }

    return userStats;
}