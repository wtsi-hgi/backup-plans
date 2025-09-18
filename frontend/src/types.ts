export type Stats = {
	ID: number;
	MTime: number;
	Files: number;
	Size: number;
}

export type Summary = {
	User: string;
	Group: string;
	Users: Record<string, Stats>;
	Groups: Record<string, Stats>;
}

export type Tree = Record<string, Summary>;

export type DirectoryRules = {
	ClaimedBy: string;
	Rules: Record<string, Rule>;
};

type BackupType = 0 | 1 | 2 | 3;

export const [BackupNone, BackupTemp, BackupIBackup, BackupManual] = Array.from({ "length": 4 }, (_, n) => n as BackupType);

export type Rule = {
	BackupType: BackupType;
	Metadata: string;
	ReviewDate: number;
	RemoveDate: number;
	Match: string;
	Frequency: number;
};

export type ReadSummary = Summary & {
	loadedData: boolean;
	loadedChildren: boolean;
	children: Record<string, ReadSummary>;
	files: number;
	size: bigint;
	rules: DirectoryRules;
}