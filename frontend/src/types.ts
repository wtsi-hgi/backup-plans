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
	Unauthorised: string[];
	CanClaim: boolean;
};

export type DirectoryRules = {
	ClaimedBy: string;
	Rules: Record<string, Rule>;
};

export type BackupType = -1 | 0 | 1 | 2 | 3;

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

export type SizeCount = {
	size: bigint;
	count: bigint;
}

export type SizeCountTime = SizeCount & {
	mtime: number;
}

export type RuleStats = Rule & SizeCountTime;

export type Directory = SizeCountTime & {
	actions: SizeCountTime[];
	users: string[];
	groups: string[];
	rules: Record<string, RuleStats[]>;
	unauthorised: boolean;
}

export type DirectoryWithChildren = Directory & {
	children: Record<string, Directory>;
	claimedBy: string;
	canClaim: boolean;
}

type ClaimedDir = DirSummary & {
	ClaimedBy: string;
}

export type ReportSummary = {
	Summaries: Record<string, ClaimedDir>;
	Rules: Record<number, Rule>;
	Directories: Record<string, number[]>;
}