import { getUserGroups } from "./rpc.js";

export const boms = new Map<string, string>(),
    owners = new Map<string, string>(),
    users = new Set<string>(),
    groups = new Set<string>(),
    userGroups = await getUserGroups();

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

for (const user of Array.from(userGroups.Users)) {
    users.add(user)
}

for (const group of Array.from(userGroups.Groups)) {
    groups.add(group)
}