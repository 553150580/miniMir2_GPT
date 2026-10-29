const fs = require('node:fs'), path = require('node:path'), crypto = require('node:crypto');
const root = process.cwd(), directory = path.join(root, '.work/pet-visual-20261029/gui/run-425e92f75bf946afb173ef23be024c21');
const relative = 'settings/v2/packages/information.json';
const source = path.join(root, 'client', relative), isolated = path.join(directory, 'client-project', relative);
const rawBefore = fs.readFileSync(source), rawAfter = fs.readFileSync(isolated), sha = value => crypto.createHash('sha256').update(value).digest('hex');
const copied = JSON.parse(fs.readFileSync(path.join(directory, 'project-copy.json'), 'utf8'));
const creator = JSON.parse(fs.readFileSync(path.join(directory, 'creator-report.json'), 'utf8'));
const before = JSON.parse(rawBefore), after = JSON.parse(rawAfter), diffs = [];
function compare(a, b, prefix = '') {
    if (a && b && typeof a === 'object' && typeof b === 'object') {
        for (const key of new Set([...Object.keys(a), ...Object.keys(b)])) compare(a[key], b[key], prefix ? prefix + '.' + key : key);
    } else if (a !== b) {
        let detail = {path: prefix, kind: 'other_change'};
        if (typeof a === 'string' && typeof b === 'string') {
            try {
                const left = new URL(a), right = new URL(b), oldSid = left.searchParams.getAll('sid'), newSid = right.searchParams.getAll('sid');
                left.searchParams.delete('sid'); right.searchParams.delete('sid');
                detail = {...detail, kind: 'URL', url_without_sid_before: left.toString(), url_without_sid_after: right.toString(),
                    only_sid_changed: left.toString() === right.toString() && oldSid.length === 1 && newSid.length === 1 && oldSid[0] !== newSid[0], sid_values_omitted: true};
            } catch {}
        }
        diffs.push(detail);
    }
}
compare(before, after);
const allowed = ['information.customSplash.customSplash.form', 'information.removeSplash.removeSplash.form'];
const out = __dirname;
fs.writeFileSync(path.join(out, 'before.raw.json'), rawBefore);
fs.writeFileSync(path.join(out, 'after.raw.json'), rawAfter);
const report = {file: 'client/' + relative, artifact_file: path.relative(root, isolated).replaceAll('\\', '/'),
    before_sha256: sha(rawBefore), after_sha256: sha(rawAfter), copied_source_sha256: copied.source_sha256[relative],
    copied_isolated_sha256: copied.project_sha256[relative], creator_after_sha256: creator.input_sha256[relative],
    original_bytes_verified: sha(rawBefore) === copied.source_sha256[relative] && sha(rawBefore) === copied.project_sha256[relative],
    creator_after_bytes_verified: sha(rawAfter) === creator.input_sha256[relative],
    leaf_changes: diffs, exact_two_url_sid_changes: diffs.length === 2 && diffs.every(value => allowed.includes(value.path) && value.only_sid_changed),
    before_top_level_keys: Object.keys(before), after_top_level_keys: Object.keys(after),
    raw_evidence: ['before.raw.json', 'after.raw.json'], sid_values_printed: false};
fs.writeFileSync(path.join(out, 'json-diff.json'), JSON.stringify(report, null, 2));
console.log(JSON.stringify(report, null, 2));
