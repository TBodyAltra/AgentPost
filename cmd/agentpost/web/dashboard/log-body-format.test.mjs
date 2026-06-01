/**
 * Unit tests for message log body formatting (mirrors index.html helpers).
 * Run: node log-body-format.test.mjs
 */

function decodeUnicodeEscapes(text) {
  const s = String(text ?? "");
  if (!/\\u[0-9a-fA-F]{4}/.test(s)) return s;
  return s.replace(/\\u([0-9a-fA-F]{4})/g, (_, hex) =>
    String.fromCodePoint(parseInt(hex, 16)));
}

function formatMessageBodyForDisplay(raw) {
  let s = String(raw ?? "").trim();
  if (!s) return "";
  s = decodeUnicodeEscapes(s);
  try {
    const parsed = JSON.parse(s);
    if (parsed !== null && typeof parsed === "object") {
      return JSON.stringify(parsed, null, 2);
    }
    if (typeof parsed === "string") {
      return decodeUnicodeEscapes(parsed);
    }
  } catch (_) {
    /* plain text */
  }
  return s;
}

function assertEqual(actual, expected, label) {
  if (actual !== expected) {
    throw new Error(`${label}\n  expected: ${JSON.stringify(expected)}\n  actual:   ${JSON.stringify(actual)}`);
  }
}

assertEqual(decodeUnicodeEscapes("\\u76ee\\u6807"), "目标", "decode plain escapes");

const jsonBody = '{"request":"\\u76ee\\u6807\\u5212\\u5b8c\\u6210"}';
const formatted = formatMessageBodyForDisplay(jsonBody);
if (!formatted.includes("目标")) {
  throw new Error(`formatted JSON should contain 目标, got: ${formatted}`);
}
if (formatted.includes("\\u76ee")) {
  throw new Error(`formatted JSON should not contain literal \\u escapes, got: ${formatted}`);
}

assertEqual(formatMessageBodyForDisplay("plain hello"), "plain hello", "plain text");
assertEqual(formatMessageBodyForDisplay(""), "", "empty");

console.log("log-body-format.test.mjs: ok");
