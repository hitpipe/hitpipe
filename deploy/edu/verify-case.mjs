import { readFile } from 'node:fs/promises';
import assert from 'node:assert/strict';

const [configFile, priorProof] = process.argv.slice(2);
if (!configFile) throw new Error('Usage: verify-case.mjs BROWSER_CONFIG [PREVIOUS_PROOF]');
const read = path => readFile(path, 'utf8').then(JSON.parse);
const config = await read(configFile);
const expected = priorProof ? await read(priorProof) : null;
const origin = new URL(config.origin);
assert.equal(origin.hostname, '127.0.0.1', 'Only the loopback school profile is supported');
assert.match(config.sessionToken, /^[A-Za-z0-9_-]{43,128}$/);
assert.match(config.workspaceId, /^[0-9a-f-]{36}$/);
const scope = '/api/v1/workspaces/' + config.workspaceId;
const headers = { Cookie: '__Host-hp_session=' + config.sessionToken };
async function get(path) {
  const response = await fetch(new URL(path, origin), {
    headers, redirect: 'error', signal: AbortSignal.timeout(5000),
  });
  assert.equal(response.status, 200, 'Failed to read ' + path);
  return response.json();
}

const cases = await get(scope + '/cases');
assert.equal(cases.items.length, 1, 'Deliberate replay must leave one case');
const item = cases.items[0];
assert.equal(item.state, 'CONFIRMED');
const detail = await get(scope + '/cases/' + item.case_id);
assert.equal(detail.case.case_id, item.case_id);
assert.equal(detail.case.operation_id, item.operation_id);
assert.equal(detail.case.state, 'CONFIRMED');
assert.equal(detail.case.outcome_knowledge, 'RESULT_VERIFIED');
assert.equal(detail.observation_complete, true);
assert.ok(Number.isFinite(Date.parse(detail.last_observation_at)), 'Missing readback time');

const observed = {
  case_id: item.case_id,
  operation_id: item.operation_id,
  case_state: detail.case.state,
  outcome_knowledge: detail.case.outcome_knowledge,
  last_observation_at: detail.last_observation_at,
};
if (expected) {
  assert.equal(observed.case_id, expected.case_id);
  assert.equal(observed.operation_id, expected.operation_id);
  assert.equal(observed.last_observation_at, expected.last_observation_at);
}
console.log(JSON.stringify(observed, null, 2));
