import { userGroups } from "./rpc.js";

export const boms = new Map<string, string>(),
    owners = new Map<string, string>();

for (const [bom, groups] of Object.entries(userGroups.BOM ?? {})) {
    for (const group of groups) {
        boms.set(group, bom);
    }
}

for (const [owner, groups] of Object.entries(userGroups.Owners ?? {})) {
    for (const group of groups) {
        owners.set(group, owner);
    }
}