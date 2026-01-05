export class BackupType extends Number {
    static BackupWarn = new BackupType(-1);
    static BackupNone = new BackupType(0);
    static BackupIBackup = new BackupType(1);
    static BackupManualIBackup = new BackupType(2);
    static BackupManualGit = new BackupType(3);
    static BackupManualUnchecked = new BackupType(4);
    static BackupManualPrefect = new BackupType(5);

    static all = [BackupType.BackupNone, BackupType.BackupIBackup, BackupType.BackupManualIBackup, BackupType.BackupManualGit, BackupType.BackupManualUnchecked, BackupType.BackupManualPrefect];
    static manual = [BackupType.BackupManualIBackup, BackupType.BackupManualGit, BackupType.BackupManualUnchecked, BackupType.BackupManualPrefect];

    static from(bt: string | number | BackupType) {
        switch (bt) {
            case +BackupType.BackupNone:
            case "nobackup":
                return BackupType.BackupNone;
            case +BackupType.BackupIBackup:
            case "backup":
                return BackupType.BackupIBackup;
            case +BackupType.BackupManualIBackup:
            case "manualibackup":
                return BackupType.BackupManualIBackup;
            case +BackupType.BackupManualGit:
            case "manualgit":
                return BackupType.BackupManualGit
            case +BackupType.BackupManualUnchecked:
            case "manualunchecked":
                return BackupType.BackupManualUnchecked;
            case +BackupType.BackupManualPrefect:
            case "manualprefect":
                return BackupType.BackupManualPrefect
        }

        throw new Error("invalid backup type");
    }

    toString() {
        switch (this) {
            case BackupType.BackupNone:
                return "nobackup";
            case BackupType.BackupIBackup:
                return "backup";
            case BackupType.BackupManualIBackup:
                return "manualibackup";
            case BackupType.BackupManualGit:
                return "manualgit";
            case BackupType.BackupManualUnchecked:
                return "manualunchecked";
            case BackupType.BackupManualPrefect:
                return "manualprefect";
        }

        return ""
    }

    optionLabel() {
        switch (+this) {
            case +BackupType.BackupNone:
                return "No Backup";
            case +BackupType.BackupIBackup:
                return "iBackup";
            case +BackupType.BackupManualIBackup:
                return "Manual Backup: iBackup";
            case +BackupType.BackupManualGit:
                return "Manual Backup: Git";
            case +BackupType.BackupManualUnchecked:
                return "Manual Backup: Unchecked";
            case +BackupType.BackupManualPrefect:
                return "Manual Backup: Prefect";
        }

        return "";
    }

    metadataLabel() {
        switch (+this) {
            case +BackupType.BackupManualIBackup:
                return "Set Name";
            case +BackupType.BackupManualGit:
                return "Git URL";
            case +BackupType.BackupManualUnchecked:
                return "Metadata";
            case +BackupType.BackupManualPrefect:
                return "Prefect URL";
        }

        return "aaa";
    }

    isManual() {
        return +this >= +BackupType.BackupManualIBackup;
    }

}

export const helpText = {
    "addEdit": ` 
    Match: The format that filenames should match for the instruction to apply.
        Eg: *, *.txt
    Override Child Rules: If selected, this rule will apply to all matched files in all child directories, regardless of their rules (unless their rules are more specific).
    Backup type: The type of backup that will/has been completed to backup all matched files.
    Metadata: For unchecked backup types, put any relevant URL/information.

    For more information, please see the relevant documentation on the 'How to backup your lustre data' page.
    `,
    "fofn": `
    Add the same rule to multiple files in one by providing a FOFN.
    Paths can be full or relative.

    For more information, please see the relevant documentation on the 'How to backup your lustre data' page.
    `,
    "dirDetail": `
    Frequency: Backup frequency in days.
    Review date: Date at which a reminder will be issued to check if the plan is still up to date.
    Remove date: Date at which the backup data will be automatically removed.

    For more information, please see the relevant documentation on the 'How to backup your lustre data' page.
    `
}