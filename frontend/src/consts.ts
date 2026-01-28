export const helpText = {
    "Match": `The format that filenames should match for the instruction to apply.
    Eg: *, *.txt`,
    "Override": `If selected, this rule will apply to all matched files in all child 
directories, regardless of their rules (unless their rules are more specific).`,
    "BackupType": `The type of backup that will/has been completed to backup all matched files.`,
    "MetadataSetName": `Provide the iBackup set name that you used to manually backup the matched files.`,
    "MetadataGit": `Provide the URL to the Git repository where you backup the matched files.`,
    "MetadataPrefect": `Provide a Prefect URL related to the matched files.`,
    "MetadataUnchecked": `Provide any information that could help you manually check the backup status of the matched files.`,
    "FOFN": `Add the same rule to multiple files in one go by providing a FOFN. Paths can be full or relative.
`,
    "Frequency": `How often all files in this directory, matched with an iBackup rule, will be backed up. Measured in days.`,
    "Review": `Date at which a reminder will be issued to check if the plan is still up to date.`,
    "Remove": `Date at which the backup data will be automatically removed.`
}

export const MainProgrammes = [
    "CASM",
    "Human Genetics",
    "ToL",
    "PAM",
    "Generative Genomics",
    "Cellular Genetics",
]

export class BackupType extends Number {
    static #stringToType = new Map<string, BackupType>();
    static #typeToString = new Map<BackupType, string>();
    static #idToType = new Map<number, BackupType>();
    static #typeToLabel = new Map<BackupType, string>();
    static #typeToMetaLabel = new Map<BackupType, string>();
    static #typeToMetaTooltip = new Map<BackupType, string>();

    static all: BackupType[] = [];
    static manual: BackupType[] = [];

    static BackupManual = new BackupType(-2, "manual");
    static BackupWarn = new BackupType(-1, "warn");
    static BackupNone = new BackupType(0, "nobackup", "No Backup");
    static BackupIBackup = new BackupType(1, "backup", "iBackup");
    static BackupManualIBackup = new BackupType(2, "manualibackup", "Manual Backup: iBackup", "Set Name", helpText.MetadataSetName);
    static BackupManualGit = new BackupType(3, "manualgit", "Manual Backup: Git", "Git URL", helpText.MetadataGit);
    static BackupManualUnchecked = new BackupType(4, "manualunchecked", "Manual Backup: Unchecked", "Metadata", helpText.MetadataUnchecked);
    static BackupManualPrefect = new BackupType(5, "manualprefect", "Manual Backup: Prefect", "Prefect URL", helpText.MetadataPrefect);

    static from(bt: string | number | BackupType) {
        const b = typeof bt === "string" ? this.#stringToType.get(bt) : this.#idToType.get(+bt);

        if (b) {
            return b
        }

        throw new Error("invalid backup type");
    }

    constructor(id: number, name: string, label?: string, metaLabel?: string, metaTooltip?: string) {
        super(id);

        BackupType.#stringToType.set(name, this);
        BackupType.#stringToType.set(id + "", this);
        BackupType.#typeToString.set(this, name);
        BackupType.#idToType.set(id, this);

        if (label) {
            BackupType.all.push(this);
            BackupType.#typeToLabel.set(this, label);

            if (label.startsWith("Manual Backup")) {
                BackupType.manual.push(this);
            }
        }

        if (metaLabel) {
            BackupType.#typeToMetaLabel.set(this, metaLabel);
        }

        if (metaTooltip) {
            BackupType.#typeToMetaTooltip.set(this, metaTooltip);
        }
    }

    toString() {
        return BackupType.#typeToString.get(this) ?? "";
    }

    optionLabel() {
        return BackupType.#typeToLabel.get(this) ?? "";
    }

    metadataLabel() {
        return BackupType.#typeToMetaLabel.get(this) ?? "";
    }

    metadataToolTip() {
        return BackupType.#typeToMetaTooltip.get(this) ?? "Error";
    }

    isBackedUp() {
        return this != BackupType.BackupNone && this !== BackupType.BackupWarn;
    }

    isManual() {
        return this === BackupType.BackupManual || BackupType.manual.includes(this);
    }
}