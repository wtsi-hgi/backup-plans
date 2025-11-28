import type { BackupType } from "./types.js";

export const [BackupNone, BackupIBackup, BackupManualIBackup, BackupManualGit, BackupManualUnchecked, BackupManualPrefect] = Array.from({ "length": 6 }, (_, n) => n as BackupType),
    BackupWarn = -1;

export const ManualBackupTypes = [BackupManualIBackup, BackupManualGit, BackupManualPrefect, BackupManualUnchecked];
export const ManualBackupStrings = ["manualibackup", "manualgit", "manualprefect", "manualunchecked"];

export const ManualBackupString = {
    "ManualBackup": "manualibackup",
    "ManualGit": "manualgit",
    "ManualPrefect": "manualprefect",
    "ManualUnchecked": "manualunchecked"
} as const;

export const ManualBackupDisplay = {
    "ManualBackup": "Manual Backup: iBackup",
    "ManualGit": "Manual Backup: Git",
    "ManualPrefect": "Manual Backup: Prefect",
    "ManualUnchecked": "Manual Backup: Unchecked"
}