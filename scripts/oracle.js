// Asserts the installed Node playwright matches the pinned interaction
// oracle in testdata/oracle-pins.json. Interaction references were recorded
// against that exact Chromium build; a different one invalidates them.
const fs = require('fs');
const path = require('path');

function assertOracle(kind) {
  const pinsPath = path.join(__dirname, '..', 'testdata', 'oracle-pins.json');
  const pins = JSON.parse(fs.readFileSync(pinsPath, 'utf8'));
  const pin = pins[kind];
  if (!pin) throw new Error(`oracle: unknown kind '${kind}' in ${pinsPath}`);
  const installed = require('playwright-core/package.json').version;
  if (installed !== pin.playwright) {
    throw new Error(`oracle: ${kind} playwright ${installed} != pinned ${pin.playwright}; run npm ci`);
  }
}

module.exports = { assertOracle };