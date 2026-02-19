export type BackupStatusCounts = {
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

export function hasFofnCountData(backups: BackupStatusCounts[]): boolean {
	return backups.some(backup => fofnCountColumns.some(column => (backup[column] ?? 0) > 0));
}
