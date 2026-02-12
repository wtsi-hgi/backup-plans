import { div, p } from "./lib/html.js";
import type { ClaimedDir, SizeCount, UserClaims } from "./types.js";
import { getClaimStats } from "./rpc.js";

const base = div();

// filter by user/group

//start with just user

// show all dirs claimed by them

getClaimStats().then(claimStats => {
    console.log("Claim stats:", claimStats);
    createClaimStats(claimStats);
});

function createClaimStats(claimStats: UserClaims[]) {
    console.log("Initialising claim stats page");


    base.appendChild(p());
}

export default Object.assign(base, {
    init: createClaimStats
});


// report only has info for backed up dirs, so i need to create a new endpoint 