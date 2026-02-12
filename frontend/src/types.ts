import { BackupType } from "./consts.js";

export type BarChartRow = {
	Programme: string;
	Fractions: number[];
	Sizes: bigint[];
};

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

export type Rule = {
	BackupType: BackupType;
	Metadata: string;
	Match: string;
	Override: boolean;
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

export type SizeCountStats = SizeCount & {
	unplannedSize: bigint;
	unplannedCount: bigint;
}

export type RuleStats = Rule & SizeCountTime;

export type Directory = SizeCountTime & {
	actions: SizeCountTime[];
	users: string[];
	groups: string[];
	rules: Record<string, RuleStats[]>;
	unauthorised: boolean;
};

export type DirectoryWithChildren = ChildDirectory & dirDetails & {
	children: Record<string, ChildDirectory>;
	claimedBy: string;
	canClaim: boolean;
};

export type ChildDirectory = Directory & {
	ruleSummaries: RuleSummary[];
}

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
	GroupBackupTypeTotals: Record<string, Record<number, SizeCount>>;
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