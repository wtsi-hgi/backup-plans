import { clearNode } from './lib/dom.js';
import { table, tbody, td, th, thead, tr, div } from './lib/html.js';
import { formatBytes } from './lib/utils.js';
import type { DirectoryWithChildren, SizeCountStats } from './types.js';

const base = tbody();

const container = div({ class: "prettyTableContainer" }, [
    table({ "class": "summary", "id": "userStats" }, [
        thead(tr([
            th("User"),
            th("Total file Size"),
            th("Total file Count"),
            th("Unplanned Size"),
            th("Unplanned count")
        ])),
        base
    ])
]);

// TODO: In practice, this table will be VERY long, should it have an internal scroll bar?

export default Object.assign(container, {
    "update": (_: string, data: DirectoryWithChildren, load: (path: string) => void) => {
        calculateUserStats(data);

        clearNode(base, Array.from(userStats).map(([user, SizeCountStats]) => {
            return tr({}, [
                td(user),
                td({ "title": SizeCountStats.size.toLocaleString() }, formatBytes(SizeCountStats.size)),
                td(SizeCountStats.count.toLocaleString()),
                td({ "title": SizeCountStats.unplannedSize.toLocaleString() }, formatBytes(SizeCountStats.unplannedSize)),
                td(SizeCountStats.unplannedCount.toLocaleString())
            ])
        }))
    }
});

const userStats = new Map<string, SizeCountStats>();

function calculateUserStats(dir: DirectoryWithChildren) {
    userStats.clear()
    for (const element of dir.ruleSummaries) {
        for (const user of element.Users) {
            const ustats = userStats.get(user.Name);
            if (!ustats) {
                const totals: SizeCountStats = user.ID === 0 ? { size: BigInt(user.Size), count: BigInt(user.Files), unplannedSize: 0n, unplannedCount: 0n }
                    : { size: BigInt(user.Size), count: BigInt(user.Files), unplannedSize: BigInt(user.Size), unplannedCount: BigInt(user.Files) };
                userStats.set(user.Name, totals);
            } else {
                if (user.ID === 0) {
                    ustats.unplannedSize += BigInt(user.Size);
                    ustats.unplannedCount += BigInt(user.Files);
                }
                ustats.size += BigInt(user.Size);
                ustats.count += BigInt(user.Files);
            }
        }
    }
}