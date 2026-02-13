import { div, h2, p, h4 } from "./lib/html.js";
import type { ClaimedDir, SizeCount, UserClaims } from "./types.js";
import { getClaimStats, user } from "./rpc.js";
import { path } from "./lib/svg.js";

const base = div({}, []);

function initialiseClaimStats() {
    console.log("Initialising claim stats page");
    getClaimStats().then(claimStats => {
        base.append(createClaimStats(claimStats));
    });
}

function createClaimStats(claimStats: UserClaims) {
    return div({},
        Object.entries(claimStats).map(([user, dirClaims]) => div({}, [
            h2(user),
            Object.entries(dirClaims).map(([path, rulestats]) => div({}, [
                h4(path),
                Array.from(rulestats).map(rule => p("BackupType: " + rule.BackupType + "Size:" + rule.size + "Count:" + rule.count))
            ]))
        ]))
    );
}

export default Object.assign(base, {
    init: initialiseClaimStats
});