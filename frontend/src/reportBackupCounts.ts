type BackupStatusCounts = {
    Uploaded?: number;
    Replaced?: number;
    Unmodified?: number;
    Missing?: number;
    Frozen?: number;
    Orphaned?: number;
    Warning?: number;
    Hardlink?: number;
    Failures?: number;
};

export const fofnCountColumns = [
    "Uploaded",
    "Replaced",
    "Unmodified",
    "Missing",
    "Frozen",
    "Orphaned",
    "Warning",
    "Hardlink"
] as const;

export function formatFofnCountDetails(backup: BackupStatusCounts) {
    return fofnCountColumns
        .filter(column => (backup[column] ?? 0) > 0)
        .map(column => `${column}: ${(backup[column] ?? 0).toLocaleString()}`)
        .join("\n");
}
