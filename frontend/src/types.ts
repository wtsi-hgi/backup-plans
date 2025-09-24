export type Stats = {
	ID: number;
	Name: string;
	MTime: number;
	Files: number;
	Size: number;
}

export type RuleSummary = {
	ID: number;
	Users: Stats[];
	Groups: Stats[];
}

export type DirSummary = {
	RuleSummaries: RuleSummary[];
	Children: Record<string, DirSummary>;
}

export type Rules = Record<string, Record<number, Rule>>;

export type Tree = DirSummary & {
	ClaimedBy: string;
	Rules: Rules;
};

export type DirectoryRules = {
	ClaimedBy: string;
	Rules: Record<string, Rule>;
};

type BackupType = -1 | 0 | 1 | 2 | 3;

export const [BackupNone, BackupTemp, BackupIBackup, BackupManual] = Array.from({ "length": 4 }, (_, n) => n as BackupType),
	BackupWarn = -1;

export type Rule = {
	BackupType: BackupType;
	Metadata: string;
	ReviewDate: number;
	RemoveDate: number;
	Match: string;
	Frequency: number;
};

export type SizeCountTime = {
	size: bigint;
	count: bigint;
	mtime: number;
}

export type RuleStats = Rule & SizeCountTime;

export type Directory = SizeCountTime & {
	actions: SizeCountTime[];
	users: string[];
	groups: string[];
	rules: Record<string, RuleStats[]>;
}

export type DirectoryWithChildren = Directory & {
	children: Record<string, Directory>;
	claimedBy: string;
}