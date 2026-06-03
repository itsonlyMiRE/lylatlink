"use strict";

const assert = require("node:assert");
const crypto = require("node:crypto");
const test = require("node:test");

const { createServer, normalizePlayerCodes } = require("./server");

test("normalizes and validates player codes", () => {
  assert.deepEqual(normalizePlayerCodes(["tafo#001", "MANG#0"]), ["MANG#0", "TAFO#001"]);
  assert.deepEqual(normalizePlayerCodes(["bad", "TAFO#001"]), ["TAFO#001"]);
  assert.deepEqual(normalizePlayerCodes(["TAFO#001", "TAFO#001"]), ["TAFO#001"]);
});

test("times out unmatched match starts", async () => {
  const fixture = await startFixture({ ttlMs: 25 });
  try {
    const response = await postJSON(fixture.url, "/match/start", startPayload("nonce-a-00000001"));
    assert.equal(response.status, "waiting");
  } finally {
    await fixture.close();
  }
});

test("pairs two distinct nonces for the same match", async () => {
  const fixture = await startFixture({ ttlMs: 500 });
  try {
    const first = postJSON(fixture.url, "/match/start", startPayload("nonce-a-00000001"));
    const second = postJSON(fixture.url, "/match/start", startPayload("nonce-b-00000002"));
    const [a, b] = await Promise.all([first, second]);

    assert.equal(a.status, "ready");
    assert.equal(b.status, "ready");
    assert.equal(a.roomToken, b.roomToken);
    assert.notEqual(a.initiator, b.initiator);
  } finally {
    await fixture.close();
  }
});

test("pairs two distinct nonces for a one-code self match", async () => {
  const fixture = await startFixture({ ttlMs: 500 });
  try {
    const first = postJSON(fixture.url, "/match/start", startPayload("nonce-a-00000001", ["TAFO#001"]));
    const second = postJSON(fixture.url, "/match/start", startPayload("nonce-b-00000002", ["TAFO#001"]));
    const [a, b] = await Promise.all([first, second]);

    assert.equal(a.status, "ready");
    assert.equal(b.status, "ready");
    assert.equal(a.roomToken, b.roomToken);
    assert.notEqual(a.initiator, b.initiator);
  } finally {
    await fixture.close();
  }
});

test("returns coturn-compatible HMAC-SHA1 TURN credentials", async () => {
  const oldSecret = process.env.TURN_SECRET;
  const oldURLs = process.env.TURN_URLS;
  const oldTTL = process.env.TURN_TTL_SECONDS;
  process.env.TURN_SECRET = "shared-turn-secret";
  process.env.TURN_URLS = "turn:turn.example:3478?transport=udp,turn:turn.example:3478?transport=tcp";
  process.env.TURN_TTL_SECONDS = "3600";

  const fixture = await startFixture({ ttlMs: 500 });
  try {
    const first = postJSON(fixture.url, "/match/start", startPayload("nonce-a-00000001"));
    const second = postJSON(fixture.url, "/match/start", startPayload("nonce-b-00000002"));
    const [a] = await Promise.all([first, second]);

    assert.deepEqual(a.turnCredentials.urls, [
      "turn:turn.example:3478?transport=udp",
      "turn:turn.example:3478?transport=tcp",
    ]);
    assert.match(a.turnCredentials.username, /^\d+:[0-9a-f-]{36}$/);

    const expectedCredential = crypto
      .createHmac("sha1", process.env.TURN_SECRET)
      .update(a.turnCredentials.username)
      .digest("base64");
    assert.equal(a.turnCredentials.credential, expectedCredential);
  } finally {
    restoreEnv("TURN_SECRET", oldSecret);
    restoreEnv("TURN_URLS", oldURLs);
    restoreEnv("TURN_TTL_SECONDS", oldTTL);
    await fixture.close();
  }
});

test("dedupes duplicate nonce submissions", async () => {
  const fixture = await startFixture({ ttlMs: 25 });
  try {
    const first = postJSON(fixture.url, "/match/start", startPayload("nonce-a-00000001"));
    const duplicate = await postJSON(fixture.url, "/match/start", startPayload("nonce-a-00000001"));
    assert.equal(duplicate.status, "waiting");
    assert.equal(duplicate.duplicate, true);

    const timedOut = await first;
    assert.equal(timedOut.status, "waiting");
  } finally {
    await fixture.close();
  }
});

test("does not let unrelated nonce end a pending match", async () => {
  const fixture = await startFixture({ ttlMs: 50 });
  try {
    const first = postJSON(fixture.url, "/match/start", startPayload("nonce-a-00000001"));
    const end = await postJSON(fixture.url, "/match/end", {
      matchId: "mode.unranked-2022-12-20T06:52:39.18-0:1:0",
      clientNonce: "nonce-z-00000026",
    });
    assert.equal(end.ok, true);

    const timedOut = await first;
    assert.equal(timedOut.status, "waiting");
  } finally {
    await fixture.close();
  }
});

test("lets submitting nonce end a pending match", async () => {
  const fixture = await startFixture({ ttlMs: 500 });
  try {
    const first = postJSON(fixture.url, "/match/start", startPayload("nonce-a-00000001"));
    const end = await postJSON(fixture.url, "/match/end", {
      matchId: "mode.unranked-2022-12-20T06:52:39.18-0:1:0",
      clientNonce: "nonce-a-00000001",
    });
    assert.equal(end.ok, true);

    const ended = await first;
    assert.equal(ended.status, "ended");
  } finally {
    await fixture.close();
  }
});

function restoreEnv(name, value) {
  if (value === undefined) {
    delete process.env[name];
  } else {
    process.env[name] = value;
  }
}

function startPayload(clientNonce, playerCodes = ["TAFO#001", "MANG#000"]) {
  return {
    matchId: "mode.unranked-2022-12-20T06:52:39.18-0:1:0",
    sessionId: "mode.unranked-2022-12-20T06:52:39.18-0",
    gameNumber: 1,
    tiebreakerNumber: 0,
    playerCodes,
    clientNonce,
  };
}

async function startFixture(options) {
  const server = createServer(options);
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const { port } = server.address();
  return {
    url: `http://127.0.0.1:${port}`,
    close: async () => {
      server.closeAll();
      await new Promise((resolve) => server.close(resolve));
    },
  };
}

async function postJSON(baseURL, path, body) {
  const response = await fetch(`${baseURL}${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  return response.json();
}
