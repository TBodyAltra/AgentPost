/**
 * Unit tests for message log body formatting (mirrors index.html helpers).
 * Run: node log-body-format.test.mjs
 */

function decodeStringEscapes(s) {
  return String(s)
    .replace(/\\u([0-9a-fA-F]{4})/g, (_, hex) => String.fromCharCode(parseInt(hex, 16)))
    .replace(/\\r\\n/g, "\n")
    .replace(/\\n/g, "\n")
    .replace(/\\r/g, "\n")
    .replace(/\\t/g, "\t");
}

function extractAgentMessageText(raw) {
  let text = decodeStringEscapes(String(raw ?? "").trim());
  if (!text) return "";
  try {
    const obj = JSON.parse(text);
    if (obj && typeof obj === "object" && !Array.isArray(obj)) {
      if (typeof obj.request === "string") return decodeStringEscapes(obj.request);
      if (typeof obj.reply === "string") return decodeStringEscapes(obj.reply);
      return JSON.stringify(obj, null, 2);
    }
  } catch (_) {
    /* plain text */
  }
  return text;
}

function assertEqual(actual, expected, label) {
  if (actual !== expected) {
    throw new Error(
      `${label}\n  expected: ${JSON.stringify(expected)}\n  actual:   ${JSON.stringify(actual)}`,
    );
  }
}

assertEqual(decodeStringEscapes("\\u76ee\\u6807"), "目标", "decode unicode escapes");
assertEqual(decodeStringEscapes("line1\\n\\n2. item"), "line1\n\n2. item", "decode newline escapes");

const jsonBody = '{"request":"\\u76ee\\u6807\\u5212\\u5b8c\\u6210"}';
const extracted = extractAgentMessageText(jsonBody);
if (!extracted.includes("目标")) {
  throw new Error(`extracted request should contain 目标, got: ${extracted}`);
}
if (extracted.includes("\\u76ee")) {
  throw new Error(`extracted request should not contain literal \\u escapes, got: ${extracted}`);
}

assertEqual(extractAgentMessageText("plain hello"), "plain hello", "plain text");
assertEqual(extractAgentMessageText(""), "", "empty");

console.log("log-body-format.test.mjs: ok");
