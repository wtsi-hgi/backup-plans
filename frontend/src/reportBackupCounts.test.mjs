import test from 'node:test';
import assert from 'node:assert/strict';
import { fofnCountColumns, hasFofnCountData } from './reportBackupCounts.ts';

test('D2 acceptance: renders extra columns when fofn counts are present', () => {
    const backup = {
        Uploaded: 5,
        Frozen: 3,
        Failures: 1
    };

    assert.equal(hasFofnCountData([backup]), true);
    assert.deepEqual(fofnCountColumns.includes('Uploaded'), true);
    assert.deepEqual(fofnCountColumns.includes('Frozen'), true);
    assert.deepEqual(backup.Uploaded, 5);
    assert.deepEqual(backup.Frozen, 3);
    assert.deepEqual(backup.Failures, 1);
});

test('D2 acceptance: no extra columns when fofn count fields are zero or absent', () => {
    assert.equal(hasFofnCountData([{}]), false);
    assert.equal(hasFofnCountData([{ Uploaded: 0, Replaced: 0, Unmodified: 0, Missing: 0, Frozen: 0, Orphaned: 0, Warning: 0, Hardlink: 0 }]), false);
});
