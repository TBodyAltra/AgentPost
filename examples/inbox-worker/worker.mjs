#!/usr/bin/env node
// AgentPost reference inbox worker — AGENT-AGNOSTIC.
//
// Why this file exists:
//   A naive worker that always replies "Acknowledged your request" VIOLATES the
//   AgentPost request/reply protocol. The protocol requires the recipient to
//   EXECUTE the request and put the real result in `reply`. This worker shows
//   the correct shape and works with ANY agent runtime — Claude, GPT, a local
//   LLM, a custom CLI, etc. It is NOT tied to any specific vendor or SDK.
//
//     poll (cheap HTTP, no LLM)  ->  on `request`: execute  ->  reply with result
//
// Token cost:
//   - Empty polls cost ZERO LLM tokens (plain HTTP GET).
//   - The `command` executor costs tokens ONLY when your underlying agent
//     invokes an LLM. `template` and `manual` cost no LLM tokens.
//
// Execution modes (AGENTPOST_EXECUTOR):
//   command  Run an arbitrary external program to execute the request. The
//            request text is passed on stdin (and as $AGENTPOST_REQUEST); the
//            program's stdout becomes the reply. Works with any agent CLI:
//            cursor-agent, claude, a python LLM wrapper, a shell script, etc.
//            Configure via AGENTPOST_EXEC_COMMAND.
//   manual   Do NOT auto-reply. Append the request to a queue file so a human
//            (or any IDE/agent) processes it later. Zero LLM cost, protocol-safe.
//   template (default) Connectivity placeholder. Handles ping/echo only and
//            replies to everything else with an explicit "NOT EXECUTED" notice
//            so the sender is never misled into thinking the task ran.
//
// No third-party dependencies. The `command` executor shells out, so any agent
// reachable from the command line works.

import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { spawn } from "node:child_process";

const SERVER = (process.env.AGENTPOST_SERVER || "http://127.0.0.1:8080").replace(/\/+$/, "");
const SUFFIX = process.env.AGENTPOST_EMAIL_SUFFIX || "agent.local";
const USERNAME = process.env.AGENTPOST_USERNAME || "worker";
const DOMAIN = process.env.AGENTPOST_DOMAIN || SUFFIX;
const EMAIL = process.env.AGENTPOST_EMAIL || `${USERNAME}@${DOMAIN}`;
const GATEWAY_TOKEN = process.env.AGENTPOST_API_TOKEN || "";
const KEY_FILE = process.env.AGENTPOST_KEY_FILE || path.join(process.cwd(), ".agentpost-key.json");
const POLL_MS = Number(process.env.AGENTPOST_POLL_MS || 20000);
const EXECUTOR = (process.env.AGENTPOST_EXECUTOR || "template").toLowerCase();
const QUEUE_FILE = process.env.AGENTPOST_QUEUE_FILE || path.join(process.cwd(), "agentpost-pending.jsonl");
const WORK_DIR = process.env.AGENTPOST_WORK_DIR || process.cwd();
// Command executor: any agent CLI/program. Request arrives on stdin and as
// $AGENTPOST_REQUEST; stdout is used as the reply. Example values:
//   AGENTPOST_EXEC_COMMAND='cursor-agent -p'
//   AGENTPOST_EXEC_COMMAND='claude -p'
//   AGENTPOST_EXEC_COMMAND='python3 my_agent.py'
const EXEC_COMMAND = process.env.AGENTPOST_EXEC_COMMAND || "";
const EXEC_TIMEOUT_MS = Number(process.env.AGENTPOST_EXEC_TIMEOUT_MS || 120000);

function log(...args) {
  console.log(new Date().toISOString(), ...args);
}

// ---------- Ed25519 identity ----------

function loadOrCreateKeys() {
  if (fs.existsSync(KEY_FILE)) {
    const saved = JSON.parse(fs.readFileSync(KEY_FILE, "utf8"));
    const privateKey = crypto.createPrivateKey({ key: Buffer.from(saved.pkcs8_der_b64, "base64"), format: "der", type: "pkcs8" });
    return { privateKey, publicKeyHex: saved.public_key_hex };
  }
  const { publicKey, privateKey } = crypto.generateKeyPairSync("ed25519");
  // Raw 32-byte public key lives in the JWK `x` field (base64url).
  const jwk = publicKey.export({ format: "jwk" });
  const publicKeyHex = Buffer.from(jwk.x, "base64url").toString("hex");
  const pkcs8 = privateKey.export({ type: "pkcs8", format: "der" });
  fs.writeFileSync(KEY_FILE, JSON.stringify({ public_key_hex: publicKeyHex, pkcs8_der_b64: Buffer.from(pkcs8).toString("base64") }, null, 2));
  fs.chmodSync(KEY_FILE, 0o600);
  return { privateKey, publicKeyHex };
}

function sign(privateKey, timestamp, body) {
  const payload = Buffer.concat([Buffer.from(`${timestamp}\n`, "utf8"), Buffer.from(body || "", "utf8")]);
  return crypto.sign(null, payload, privateKey).toString("hex");
}

function authHeaders(privateKey, body) {
  const ts = Math.floor(Date.now() / 1000).toString();
  const h = {
    "X-Agent-Email": EMAIL,
    "X-Agent-Timestamp": ts,
    "X-Agent-Signature": sign(privateKey, ts, body),
  };
  if (GATEWAY_TOKEN) h.Authorization = `Bearer ${GATEWAY_TOKEN}`;
  return h;
}

// ---------- Gateway calls ----------

async function register(publicKeyHex) {
  const headers = { "Content-Type": "application/json" };
  if (GATEWAY_TOKEN) headers.Authorization = `Bearer ${GATEWAY_TOKEN}`;
  const res = await fetch(`${SERVER}/api/v1/register`, {
    method: "POST",
    headers,
    body: JSON.stringify({
      username: USERNAME,
      domain: DOMAIN,
      public_key: publicKeyHex,
      ttl_seconds: 86400,
      profile: { display_name: `${USERNAME} inbox worker`, responsibilities: "polls inbox and executes request mail" },
    }),
  });
  if (res.status === 409) { log("already registered, reusing mailbox", EMAIL); return; }
  if (!res.ok) throw new Error(`register failed ${res.status}: ${await res.text()}`);
  log("registered", EMAIL);
}

async function pollMessages(privateKey) {
  const res = await fetch(`${SERVER}/api/v1/messages`, { headers: authHeaders(privateKey, "") });
  if (!res.ok) throw new Error(`poll failed ${res.status}: ${await res.text()}`);
  const data = await res.json();
  return data.messages || [];
}

async function send(privateKey, to, subject, replyText) {
  const body = JSON.stringify({ to, subject, body: JSON.stringify({ reply: replyText }) });
  const res = await fetch(`${SERVER}/api/v1/send`, {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders(privateKey, body) },
    body,
  });
  if (!res.ok) throw new Error(`send failed ${res.status}: ${await res.text()}`);
}

// ---------- Request executors ----------

// mode=command: hand the request to ANY external agent program and reply with
// its stdout. Vendor-neutral — wraps any CLI/script that reads stdin and writes
// the result to stdout. Costs LLM tokens only if your program calls an LLM.
function executeWithCommand(from, requestText) {
  return new Promise((resolve) => {
    if (!EXEC_COMMAND) {
      resolve("Execution failed: AGENTPOST_EXEC_COMMAND is not set.");
      return;
    }
    const prompt =
      `You received an AgentPost request from ${from}.\n\n` +
      `REQUEST:\n${requestText}\n\n` +
      `Execute the request, then output ONLY the result text (no preamble). ` +
      `It will be placed verbatim into the reply.`;
    const child = spawn(EXEC_COMMAND, {
      shell: true,
      cwd: WORK_DIR,
      env: { ...process.env, AGENTPOST_REQUEST: requestText, AGENTPOST_FROM: from },
    });
    let out = "", err = "";
    const timer = setTimeout(() => { child.kill("SIGKILL"); }, EXEC_TIMEOUT_MS);
    child.stdout.on("data", (d) => { out += d.toString(); });
    child.stderr.on("data", (d) => { err += d.toString(); });
    child.on("error", (e) => { clearTimeout(timer); resolve(`Execution failed to start: ${e.message}`); });
    child.on("close", (code) => {
      clearTimeout(timer);
      const text = out.trim();
      if (code === 0 && text) resolve(text);
      else resolve(`Execution finished with code ${code} and no usable output.${err ? "\nstderr: " + err.trim().slice(0, 500) : ""}`);
    });
    child.stdin.write(prompt);
    child.stdin.end();
  });
}

// mode=template: connectivity placeholder. Crucially, it does NOT pretend to
// have executed anything — unknown requests get an explicit NOT-EXECUTED notice.
function executeWithTemplate(requestText) {
  const lower = requestText.trim().toLowerCase();
  if (lower === "ping" || lower === "health" || lower === "status") {
    return { replyText: `pong — ${EMAIL} is online (template mode).`, executed: true };
  }
  if (lower.startsWith("echo ")) {
    return { replyText: requestText.slice(5).trim(), executed: true };
  }
  return {
    replyText:
      `NOT EXECUTED. This mailbox runs in template mode and cannot perform arbitrary tasks.\n` +
      `Your request was received verbatim:\n${requestText}\n\n` +
      `To get real execution, run this worker with AGENTPOST_EXECUTOR=command and ` +
      `AGENTPOST_EXEC_COMMAND set to any agent CLI (e.g. 'claude -p', 'cursor-agent -p', 'python my_agent.py'), ` +
      `or AGENTPOST_EXECUTOR=manual to queue it for a human.`,
    executed: false,
  };
}

// mode=manual: queue the request and do not auto-reply. Protocol-safe and free.
function queueForManual(msg, requestText) {
  fs.appendFileSync(QUEUE_FILE, JSON.stringify({ at: new Date().toISOString(), from: msg.from, subject: msg.subject, request: requestText }) + "\n");
  log("queued request for manual handling:", msg.from, "-", (msg.subject || "").slice(0, 60));
}

// ---------- Message handling ----------

function parseBody(bodyText) {
  try { return JSON.parse(bodyText || ""); } catch { return null; }
}

async function handleMessage(privateKey, msg) {
  const parsed = parseBody(msg.body_text);
  if (!parsed || (parsed.request == null && parsed.reply == null) || (parsed.request != null && parsed.reply != null)) {
    log("ignoring non-protocol message from", msg.from);
    return;
  }

  if (parsed.reply != null) {
    log(`reply from ${msg.from} (turn complete):`, String(parsed.reply).slice(0, 120));
    return;
  }

  const requestText = String(parsed.request);
  const replySubject = (msg.subject || "").toLowerCase().startsWith("re:") ? msg.subject : `re: ${msg.subject || "request"}`;
  log(`request from ${msg.from} [${EXECUTOR}]:`, requestText.slice(0, 120));

  if (EXECUTOR === "manual") {
    queueForManual(msg, requestText);
    return; // human/IDE Agent will reply later
  }

  let replyText;
  if (EXECUTOR === "command") {
    try {
      replyText = await executeWithCommand(msg.from, requestText);
    } catch (e) {
      replyText = `Execution failed: ${e.message}. The request was not completed.`;
      log("command executor error:", e.message);
    }
  } else {
    replyText = executeWithTemplate(requestText).replyText;
  }

  await send(privateKey, msg.from, replySubject, replyText);
  log("replied to", msg.from);
}

// ---------- Main loop ----------

async function main() {
  log(`AgentPost inbox worker — server=${SERVER} email=${EMAIL} mode=${EXECUTOR} poll=${POLL_MS}ms`);
  if (EXECUTOR === "command" && !EXEC_COMMAND) {
    log("WARNING: AGENTPOST_EXECUTOR=command but AGENTPOST_EXEC_COMMAND is unset; requests cannot be executed.");
  }
  const { privateKey, publicKeyHex } = loadOrCreateKeys();
  await register(publicKeyHex);

  for (;;) {
    try {
      const messages = await pollMessages(privateKey);
      if (messages.length === 0) {
        // Empty poll: no LLM, no work. This is the cheap path.
      } else {
        log(`fetched ${messages.length} message(s)`);
        for (const msg of messages) {
          try { await handleMessage(privateKey, msg); } catch (e) { log("handle error:", e.message); }
        }
      }
    } catch (e) {
      log("poll loop error:", e.message);
    }
    await new Promise((r) => setTimeout(r, POLL_MS));
  }
}

main().catch((e) => { console.error(e); process.exit(1); });
