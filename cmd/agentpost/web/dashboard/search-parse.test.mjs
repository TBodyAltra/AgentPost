/**
 * Unit tests for dashboard search parsing (run: node search-parse.test.mjs).
 */
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import vm from "node:vm";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const html = fs.readFileSync(path.join(__dirname, "index.html"), "utf8");
const m = html.match(/<script>\s*([\s\S]*?)\s*<\/script>/);
if (!m) throw new Error("dashboard script not found");
const script = m[1];

const start = script.indexOf("function regexTestFlags");
const end = script.indexOf("function sortEmailsForMatrix");
if (start < 0 || end < 0) throw new Error("search helpers not found in dashboard script");

const chunk = script.slice(start, end);
const sandbox = {};
vm.createContext(sandbox);
vm.runInContext(
  chunk +
    `
;globalThis.parseSearchQuery = parseSearchQuery;
globalThis.mailboxMatchesSearch = mailboxMatchesSearch;
`,
  sandbox,
  { filename: "search-parse.js" }
);

const { parseSearchQuery, mailboxMatchesSearch } = sandbox;

assert.equal(parseSearchQuery("").kind, "none");
assert.equal(parseSearchQuery("beta").kind, "text");
assert.ok(parseSearchQuery("beta").test("beta@agent.test"));

const anchored = parseSearchQuery("/^alpha@agent\\.test$/");
assert.equal(anchored.kind, "regex");
assert.ok(anchored.test("alpha@agent.test"));
assert.ok(!anchored.test("beta@agent.test"));

const incomplete = parseSearchQuery("/policy_bl");
assert.equal(incomplete.kind, "incomplete");

const escaped = parseSearchQuery("/^a\\/b@x\\.com$/i");
assert.equal(escaped.kind, "regex");
assert.ok(escaped.test("a/b@x.com"));

const globalSafe = parseSearchQuery("/a/g");
assert.equal(globalSafe.kind, "regex");
assert.ok(globalSafe.test("alpha@x.com"));
assert.ok(globalSafe.test("alpha@x.com"));

assert.ok(
  mailboxMatchesSearch("gamma@agent.test", "agent.test", parseSearchQuery("/policy_bl/"), {
    notes: "policy_blocked",
  })
);
assert.ok(
  !mailboxMatchesSearch("beta@agent.test", "agent.test", parseSearchQuery("/^gamma$/"), null)
);

console.log("search-parse.test.mjs: ok");
