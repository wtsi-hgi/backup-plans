import test from 'node:test';
import assert from 'node:assert/strict';
import { formatFofnCountDetails } from './reportBackupCounts.js';

test('D2 acceptance: formats non-zero fofn counts for hover details', () => {
    assert.equal(
        formatFofnCountDetails({ Uploaded: 5, Frozen: 3, Failures: 1 }),
        'Uploaded: 5\nFrozen: 3'
    );
});

test('D2 acceptance: returns empty hover details when fofn counts are zero or absent', () => {
    assert.equal(formatFofnCountDetails({}), '');
    assert.equal(formatFofnCountDetails({ Uploaded: 0, Replaced: 0, Unmodified: 0, Missing: 0, Frozen: 0, Orphaned: 0, Warning: 0, Hardlink: 0 }), '');
});
