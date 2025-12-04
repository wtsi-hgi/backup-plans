import { clearNode } from './lib/dom.js';
import { table, tbody, td, th, thead, tr, div } from './lib/html.js';
import type { DirectoryWithChildren, SizeCount } from './types.js';

const base = tbody();

const container = div({ class: "prettyTableContainer" }, [
    table({ "class": "summary", "id": "userStats" }, [
        thead(tr([
            th("User"),
            th("File Size"),
            th("File Count")
        ])),
        base
    ])
]);

export default Object.assign(container, {
    "update": (_: string, data: DirectoryWithChildren, load: (path: string) => void) => {
        calculateUserStats(data);

        clearNode(base, Array.from(userStats).map(([user, SizeCount]) => {
            return tr({}, [
                td(user),
                td(SizeCount.size.toLocaleString()), // TODO: Where are the units? what if the size is given in different units? does this happen?
                td(SizeCount.count.toLocaleString())
            ])
        }))
    }
});

const userStats = new Map<string, SizeCount>();

function calculateUserStats(dir: DirectoryWithChildren) {
    console.log("Rule summaries: ", dir.ruleSummaries);
    for (const element of dir.ruleSummaries) {
        for (const user of element.Users) {
            const ustats = userStats.get(user.Name);
            if (!ustats) {
                const totals: SizeCount = { size: BigInt(user.Size), count: BigInt(user.Files) };
                userStats.set(user.Name, totals);
            } else {
                ustats.size += BigInt(user.Size);
                ustats.count += BigInt(user.Files);
            }
        }
    }
}