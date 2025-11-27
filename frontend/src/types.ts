export type Stats = {
	ID: number;
	Name: string;
	MTime: number;
	Files: number;
	Size: number;
};

export type RuleSummary = {
	ID: number;
	Users: Stats[];
	Groups: Stats[];
};

export type DirSummary = {
	RuleSummaries: RuleSummary[];
	ClaimedBy: string;
	Children: Record<string, DirSummary>;
};

export type Rules = Record<string, Record<number, Rule>>;

export type Tree = DirSummary & dirDetails & {
	ClaimedBy: string;
	Rules: Rules;
	Unauthorised: string[];
	CanClaim: boolean;
};

export type DirectoryRules = {
	ClaimedBy: string;
	Rules: Record<string, Rule>;
};

export type BackupType = -1 | 0 | 1 | 2 | 3 | 4 | 5;

export const [BackupNone, BackupIBackup, BackupManualIBackup, BackupManualGit, BackupManualUnchecked, BackupManualPrefect] = Array.from({ "length": 7 }, (_, n) => n as BackupType),
	BackupWarn = -1;

export type Rule = {
	BackupType: BackupType;
	Metadata: string;
	Match: string;
};

export type dirDetails = {
	Frequency: number;
	ReviewDate: number;
	RemoveDate: number;
};

export type SizeCount = {
	size: bigint;
	count: bigint;
};

export type SizeCountTime = SizeCount & {
	mtime: number;
};

export type RuleStats = Rule & SizeCountTime;

export type Directory = SizeCountTime & {
	actions: SizeCountTime[];
	users: string[];
	groups: string[];
	rules: Record<string, RuleStats[]>;
	unauthorised: boolean;
};

export type DirectoryWithChildren = Directory & dirDetails & {
	children: Record<string, Directory>;
	claimedBy: string;
	canClaim: boolean;
};

export type ClaimedDir = DirSummary & {
	User: string;
	Group: string;
	ClaimedBy: string;
	Children: Record<string, ClaimedDir>;
};

export type ReportSummary = {
	Summaries: Record<string, ClaimedDir>;
	Rules: Record<number, Rule>;
	Directories: Record<string, number[]>;
	BackupStatus: Record<string, BackupStatus>;
};

export type BackupStatus = {
	LastSuccess: string;
	Name: string;
	Requester: string;
	Failures: number;
};

export type UserGroups = {
	Users: string[];
	Groups: string[];
	Owners: Record<string, string[]>;
	BOM: Record<string, string[]>;
}