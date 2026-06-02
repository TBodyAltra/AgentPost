/**
 * Unit tests for message log body formatting (mirrors index.html helpers).
 * Run: node log-body-format.test.mjs
 */

function decodeStringEscapes(s) {
  let out = String(s);
  for (let pass = 0; pass < 3; pass++) {
    const next = out
      .replace(/\\u([0-9a-fA-F]{4})/g, (_, hex) => String.fromCharCode(parseInt(hex, 16)))
      .replace(/\\r\\n/g, "\n")
      .replace(/\\n/g, "\n")
      .replace(/\\r/g, "\n")
      .replace(/\\t/g, "\t");
    if (next === out) break;
    out = next;
  }
  return out;
}

function finalizeAgentPlainText(s) {
  let text = decodeStringEscapes(String(s ?? "").trim());
  if (!text) return "";
  const trimmed = text.trim();
  if (trimmed.startsWith("{") || trimmed.startsWith("[")) {
    try {
      const inner = JSON.parse(trimmed);
      if (typeof inner === "string") return finalizeAgentPlainText(inner);
      if (inner && typeof inner === "object" && !Array.isArray(inner)) {
        if (typeof inner.request === "string") return finalizeAgentPlainText(inner.request);
        if (typeof inner.reply === "string") return finalizeAgentPlainText(inner.reply);
      }
    } catch (_) {
      /* keep text */
    }
  }
  return text;
}

function extractAgentMessageText(raw) {
  let text = String(raw ?? "").trim();
  if (!text) return "";
  for (let pass = 0; pass < 4; pass++) {
    text = decodeStringEscapes(text);
    if (!text) return "";
    try {
      const parsed = JSON.parse(text);
      if (typeof parsed === "string") {
        if (parsed === text) break;
        text = parsed;
        continue;
      }
      if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
        if (typeof parsed.request === "string") return finalizeAgentPlainText(parsed.request);
        if (typeof parsed.reply === "string") return finalizeAgentPlainText(parsed.reply);
        return JSON.stringify(parsed, null, 2);
      }
    } catch (_) {
      break;
    }
  }
  return finalizeAgentPlainText(text);
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

const listBody = '{"request":"done\\n\\n1. first\\n2. second"}';
const listText = extractAgentMessageText(listBody);
if (!listText.includes("1. first") || !listText.includes("2. second")) {
  throw new Error(`list body should decode newlines, got: ${listText}`);
}
if (listText.includes("\\n")) {
  throw new Error(`list body should not show literal \\\\n, got: ${listText}`);
}

const doubleEncoded = '"{\\"request\\":\\"\\\\u76ee\\\\u6807\\"}"';
const fromDouble = extractAgentMessageText(doubleEncoded);
if (!fromDouble.includes("目")) {
  throw new Error(`double-encoded body should unwrap to 目, got: ${fromDouble}`);
}

assertEqual(extractAgentMessageText("plain hello"), "plain hello", "plain text");
assertEqual(extractAgentMessageText(""), "", "empty");

console.log("log-body-format.test.mjs: ok");
