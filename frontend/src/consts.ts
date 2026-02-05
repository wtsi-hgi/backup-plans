import { getMainProgrammes } from "./rpc.js";
// export class BackupType extends Number {
//     static BackupWarn = new BackupType(-1);
//     static BackupNone = new BackupType(0);
//     static BackupIBackup = new BackupType(1);
//     static BackupManualIBackup = new BackupType(2);
//     static BackupManualGit = new BackupType(3);
//     static BackupManualUnchecked = new BackupType(4);
//     static BackupManualPrefect = new BackupType(5);

//     static all = [BackupType.BackupNone, BackupType.BackupIBackup, BackupType.BackupManualIBackup, BackupType.BackupManualGit, BackupType.BackupManualUnchecked, BackupType.BackupManualPrefect];
//     static manual = [BackupType.BackupManualIBackup, BackupType.BackupManualGit, BackupType.BackupManualUnchecked, BackupType.BackupManualPrefect];

//     static from(bt: string | number | BackupType) {
//         switch (bt) {
//             case +BackupType.BackupNone:
//             case "nobackup":
//                 return BackupType.BackupNone;
//             case +BackupType.BackupIBackup:
//             case "backup":
//                 return BackupType.BackupIBackup;
//             case +BackupType.BackupManualIBackup:
//             case "manualibackup":
//                 return BackupType.BackupManualIBackup;
//             case +BackupType.BackupManualGit:
//             case "manualgit":
//                 return BackupType.BackupManualGit
//             case +BackupType.BackupManualUnchecked:
//             case "manualunchecked":
//                 return BackupType.BackupManualUnchecked;
//             case +BackupType.BackupManualPrefect:
//             case "manualprefect":
//                 return BackupType.BackupManualPrefect
//         }

//         throw new Error("invalid backup type");
//     }

//     toString() {
//         switch (this) {
//             case BackupType.BackupNone:
//                 return "nobackup";
//             case BackupType.BackupIBackup:
//                 return "backup";
//             case BackupType.BackupManualIBackup:
//                 return "manualibackup";
//             case BackupType.BackupManualGit:
//                 return "manualgit";
//             case BackupType.BackupManualUnchecked:
//                 return "manualunchecked";
//             case BackupType.BackupManualPrefect:
//                 return "manualprefect";
//         }

//         return ""
//     }

//     optionLabel() {
//         switch (+this) {
//             case +BackupType.BackupNone:
//                 return "No Backup";
//             case +BackupType.BackupIBackup:
//                 return "iBackup";
//             case +BackupType.BackupManualIBackup:
//                 return "Manual Backup: iBackup";
//             case +BackupType.BackupManualGit:
//                 return "Manual Backup: Git";
//             case +BackupType.BackupManualUnchecked:
//                 return "Manual Backup: Unchecked";
//             case +BackupType.BackupManualPrefect:
//                 return "Manual Backup: Prefect";
//         }

//         return "";
//     }

//     metadataLabel() {
//         switch (+this) {
//             case +BackupType.BackupManualIBackup:
//                 return "Set Name";
//             case +BackupType.BackupManualGit:
//                 return "Git URL";
//             case +BackupType.BackupManualUnchecked:
//                 return "Metadata";
//             case +BackupType.BackupManualPrefect:
//                 return "Prefect URL";
//         }

//         return "";
//     }

//     metadataToolTip() {
//         switch (+this) {
//             case +BackupType.BackupManualIBackup:
//                 return helpText.MetadataSetName;
//             case +BackupType.BackupManualGit:
//                 return helpText.MetadataGit;
//             case +BackupType.BackupManualUnchecked:
//                 return helpText.MetadataUnchecked;
//             case +BackupType.BackupManualPrefect:
//                 return helpText.MetadataPrefect;
//         }

//         return "Error";
//     }

//     isManual() {
//         return +this >= +BackupType.BackupManualIBackup;
//     }

// }

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

export const MainProgrammes = await getMainProgrammes();
