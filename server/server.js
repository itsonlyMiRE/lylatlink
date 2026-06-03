"use strict";

const crypto = require("node:crypto");
const http = require("node:http");
const { URL } = require("node:url");

const DEFAULT_TTL_MS = 30_000;
const DEFAULT_ROOM_TTL_MS = 4 * 60 * 60 * 1000;

function createServer(options = {}) {
  const state = {
    ttlMs: options.ttlMs ?? DEFAULT_TTL_MS,
    roomTtlMs: options.roomTtlMs ?? DEFAULT_ROOM_TTL_MS,
    pending: new Map(),
    rooms: new Map(),
    rate: new Map(),
  };

  const server = http.createServer((req, res) => {
    withErrors(res, () => routeHTTP(state, req, res));
  });

  server.on("upgrade", (req, socket) => {
    withSocketErrors(socket, () => routeWebSocket(state, req, socket));
  });

  server.state = state;
  server.closeAll = () => {
    for (const room of state.rooms.values()) {
      closeRoom(state, room, "server_shutdown");
    }
    for (const pending of state.pending.values()) {
      clearTimeout(pending.timer);
      for (const waiter of pending.waiters.values()) {
        sendJSON(waiter.res, 200, { status: "waiting" });
      }
    }
    state.pending.clear();
  };
  return server;
}

async function routeHTTP(state, req, res) {
  if (!rateLimit(state, req)) {
    sendJSON(res, 429, { error: "rate_limited" });
    return;
  }

  const url = new URL(req.url, "http://127.0.0.1");
  if (req.method === "GET" && url.pathname === "/healthz") {
    sendJSON(res, 200, { ok: true });
    return;
  }
  if (req.method === "POST" && url.pathname === "/match/start") {
    await handleMatchStart(state, req, res);
    return;
  }
  if (req.method === "POST" && url.pathname === "/match/end") {
    await handleMatchEnd(state, req, res);
    return;
  }
  sendJSON(res, 404, { error: "not_found" });
}

async function handleMatchStart(state, req, res) {
  const body = await readJSON(req);
  const validation = validateStart(body);
  if (validation.error) {
    sendJSON(res, 400, { error: validation.error });
    return;
  }

  const start = validation.value;
  const key = pendingKey(start.matchId, start.playerCodes);
  const existingRoom = [...state.rooms.values()].find((room) => room.key === key);
  if (existingRoom) {
    if (!existingRoom.clientNonces.includes(start.clientNonce)) {
      sendJSON(res, 409, { error: "room_full" });
      return;
    }
    sendJSON(res, 200, readyPayload(existingRoom, start.clientNonce));
    return;
  }

  let pending = state.pending.get(key);
  if (!pending) {
    pending = {
      key,
      matchId: start.matchId,
      sessionId: start.sessionId,
      gameNumber: start.gameNumber,
      tiebreakerNumber: start.tiebreakerNumber,
      playerCodes: start.playerCodes,
      submissions: new Map(),
      waiters: new Map(),
      timer: setTimeout(() => expirePending(state, key), state.ttlMs),
    };
    state.pending.set(key, pending);
  }

  if (pending.submissions.has(start.clientNonce)) {
    sendJSON(res, 200, { status: "waiting", duplicate: true });
    return;
  }

  pending.submissions.set(start.clientNonce, start);
  pending.waiters.set(start.clientNonce, { res });

  if (pending.submissions.size >= 2) {
    const room = createRoom(state, pending);
    state.pending.delete(key);
    clearTimeout(pending.timer);
    for (const nonce of pending.waiters.keys()) {
      const waiter = pending.waiters.get(nonce);
      sendJSON(waiter.res, 200, readyPayload(room, nonce));
    }
  }
}

async function handleMatchEnd(state, req, res) {
  const body = await readJSON(req);
  if (!isNonEmpty(body.matchId) || !isNonce(body.clientNonce)) {
    sendJSON(res, 400, { error: "invalid_match_end" });
    return;
  }

  for (const room of state.rooms.values()) {
    if (room.matchId === body.matchId && room.clientNonces.includes(body.clientNonce)) {
      closeRoom(state, room, "match_end");
    }
  }

  for (const [key, pending] of state.pending.entries()) {
    if (pending.matchId === body.matchId) {
      clearTimeout(pending.timer);
      for (const waiter of pending.waiters.values()) {
        sendJSON(waiter.res, 200, { status: "ended" });
      }
      state.pending.delete(key);
    }
  }

  sendJSON(res, 200, { ok: true });
}

function routeWebSocket(state, req, socket) {
  const url = new URL(req.url, "http://127.0.0.1");
  if (url.pathname !== "/signal") {
    socket.destroy();
    return;
  }

  const token = url.searchParams.get("roomToken");
  const room = state.rooms.get(token);
  if (!room) {
    socket.destroy();
    return;
  }

  const key = req.headers["sec-websocket-key"];
  if (!key) {
    socket.destroy();
    return;
  }

  const accept = crypto
    .createHash("sha1")
    .update(`${key}258EAFA5-E914-47DA-95CA-C5AB0DC85B11`)
    .digest("base64");

  socket.write(
    [
      "HTTP/1.1 101 Switching Protocols",
      "Upgrade: websocket",
      "Connection: Upgrade",
      `Sec-WebSocket-Accept: ${accept}`,
      "",
      "",
    ].join("\r\n")
  );

  room.sockets.add(socket);
  socket._lylatBuffer = Buffer.alloc(0);
  for (const message of room.backlog) {
    sendWebSocketText(socket, message);
  }

  socket.on("data", (chunk) => {
    socket._lylatBuffer = Buffer.concat([socket._lylatBuffer, chunk]);
    parseFrames(socket, (message) => {
      room.backlog.push(message);
      if (room.backlog.length > 128) {
        room.backlog.shift();
      }
      for (const peer of room.sockets) {
        if (peer !== socket && !peer.destroyed) {
          sendWebSocketText(peer, message);
        }
      }
    });
  });

  socket.on("close", () => {
    room.sockets.delete(socket);
    scheduleEmptyRoomClose(state, room);
  });
  socket.on("error", () => {
    room.sockets.delete(socket);
    scheduleEmptyRoomClose(state, room);
  });
}

function createRoom(state, pending) {
  const clientNonces = [...pending.submissions.keys()].sort();
  const room = {
    key: pending.key,
    token: crypto.randomBytes(24).toString("base64url"),
    matchId: pending.matchId,
    sessionId: pending.sessionId,
    gameNumber: pending.gameNumber,
    tiebreakerNumber: pending.tiebreakerNumber,
    playerCodes: pending.playerCodes,
    clientNonces,
    sockets: new Set(),
    backlog: [],
    expires: null,
    emptyClose: null,
  };
  room.expires = setTimeout(() => closeRoom(state, room, "expired"), state.roomTtlMs);
  state.rooms.set(room.token, room);
  return room;
}

function readyPayload(room, nonce) {
  return {
    status: "ready",
    roomToken: room.token,
    signalUrl: `/signal?roomToken=${encodeURIComponent(room.token)}`,
    initiator: nonce === room.clientNonces[0],
    turnCredentials: makeTurnCredentials(),
  };
}

function makeTurnCredentials() {
  const secret = process.env.TURN_SECRET;
  const urls = (process.env.TURN_URLS || "")
    .split(",")
    .map((url) => url.trim())
    .filter(Boolean);
  if (!secret || urls.length === 0) {
    return {};
  }
  const ttl = Number(process.env.TURN_TTL_SECONDS || 3600);
  const timestamp = Math.floor(Date.now() / 1000) + ttl;
  const username = `${timestamp}:${crypto.randomUUID()}`;
  const credential = crypto.createHmac("sha1", secret).update(username).digest("base64");
  return { urls, username, credential };
}

function expirePending(state, key) {
  const pending = state.pending.get(key);
  if (!pending) {
    return;
  }
  for (const waiter of pending.waiters.values()) {
    sendJSON(waiter.res, 200, { status: "waiting" });
  }
  state.pending.delete(key);
}

function closeRoom(state, room, reason) {
  clearTimeout(room.expires);
  clearTimeout(room.emptyClose);
  for (const socket of room.sockets) {
    try {
      sendWebSocketText(socket, JSON.stringify({ type: "room_closed", reason }));
      socket.end();
    } catch {
      socket.destroy();
    }
  }
  room.sockets.clear();
  state.rooms.delete(room.token);
}

function scheduleEmptyRoomClose(state, room) {
  if (room.sockets.size > 0 || room.emptyClose) {
    return;
  }
  room.emptyClose = setTimeout(() => {
    if (room.sockets.size === 0) {
      closeRoom(state, room, "empty");
    }
  }, 5000);
}

function validateStart(body) {
  if (!isNonEmpty(body.matchId) || !isNonEmpty(body.sessionId)) {
    return { error: "invalid_match" };
  }
  if (!Number.isInteger(body.gameNumber) || !Number.isInteger(body.tiebreakerNumber)) {
    return { error: "invalid_game_number" };
  }
  if (!isNonce(body.clientNonce)) {
    return { error: "invalid_client_nonce" };
  }
  const playerCodes = normalizePlayerCodes(body.playerCodes);
  if (playerCodes.length !== 2) {
    return { error: "invalid_player_codes" };
  }
  return {
    value: {
      matchId: body.matchId,
      sessionId: body.sessionId,
      gameNumber: body.gameNumber,
      tiebreakerNumber: body.tiebreakerNumber,
      playerCodes,
      clientNonce: body.clientNonce,
    },
  };
}

function normalizePlayerCodes(codes) {
  if (!Array.isArray(codes)) {
    return [];
  }
  const normalized = [...new Set(codes.map(normalizeCode).filter(Boolean))].sort();
  return normalized.filter((code) => /^[A-Z0-9]{2,8}#[0-9]{1,3}$/.test(code));
}

function normalizeCode(code) {
  return typeof code === "string" ? code.trim().toUpperCase() : "";
}

function isNonEmpty(value) {
  return typeof value === "string" && value.trim().length > 0;
}

function isNonce(value) {
  return typeof value === "string" && /^[a-zA-Z0-9_-]{16,128}$/.test(value);
}

function pendingKey(matchId, playerCodes) {
  return `${matchId}|${playerCodes.join(",")}`;
}

async function readJSON(req) {
  const chunks = [];
  let total = 0;
  for await (const chunk of req) {
    total += chunk.length;
    if (total > 1024 * 1024) {
      throw httpError(413, "payload_too_large");
    }
    chunks.push(chunk);
  }
  try {
    return JSON.parse(Buffer.concat(chunks).toString("utf8") || "{}");
  } catch {
    throw httpError(400, "invalid_json");
  }
}

function sendJSON(res, statusCode, payload) {
  if (res.writableEnded) {
    return;
  }
  const body = JSON.stringify(payload);
  res.writeHead(statusCode, {
    "Content-Type": "application/json",
    "Content-Length": Buffer.byteLength(body),
  });
  res.end(body);
}

function parseFrames(socket, onMessage) {
  let buf = socket._lylatBuffer;
  while (buf.length >= 2) {
    const first = buf[0];
    const second = buf[1];
    const opcode = first & 0x0f;
    const masked = (second & 0x80) !== 0;
    let length = second & 0x7f;
    let offset = 2;

    if (length === 126) {
      if (buf.length < offset + 2) break;
      length = buf.readUInt16BE(offset);
      offset += 2;
    } else if (length === 127) {
      if (buf.length < offset + 8) break;
      const high = buf.readUInt32BE(offset);
      const low = buf.readUInt32BE(offset + 4);
      if (high !== 0) {
        socket.destroy();
        return;
      }
      length = low;
      offset += 8;
    }

    const maskOffset = offset;
    if (masked) {
      offset += 4;
    }
    if (buf.length < offset + length) {
      break;
    }

    let payload = buf.subarray(offset, offset + length);
    if (masked) {
      const mask = buf.subarray(maskOffset, maskOffset + 4);
      payload = Buffer.from(payload.map((byte, i) => byte ^ mask[i % 4]));
    }
    buf = buf.subarray(offset + length);

    if (opcode === 0x8) {
      socket.end();
      continue;
    }
    if (opcode === 0x1) {
      onMessage(payload.toString("utf8"));
    }
  }
  socket._lylatBuffer = buf;
}

function sendWebSocketText(socket, text) {
  const payload = Buffer.from(text);
  let header;
  if (payload.length < 126) {
    header = Buffer.from([0x81, payload.length]);
  } else if (payload.length < 65536) {
    header = Buffer.alloc(4);
    header[0] = 0x81;
    header[1] = 126;
    header.writeUInt16BE(payload.length, 2);
  } else {
    header = Buffer.alloc(10);
    header[0] = 0x81;
    header[1] = 127;
    header.writeUInt32BE(0, 2);
    header.writeUInt32BE(payload.length, 6);
  }
  socket.write(Buffer.concat([header, payload]));
}

function rateLimit(state, req) {
  const ip = req.socket.remoteAddress || "unknown";
  const now = Date.now();
  const windowMs = 60_000;
  const max = 30;
  const item = state.rate.get(ip) || { count: 0, resetAt: now + windowMs };
  if (now > item.resetAt) {
    item.count = 0;
    item.resetAt = now + windowMs;
  }
  item.count += 1;
  state.rate.set(ip, item);
  return item.count <= max;
}

function withErrors(res, fn) {
  Promise.resolve()
    .then(fn)
    .catch((error) => {
      const status = error.statusCode || 500;
      sendJSON(res, status, { error: error.message || "server_error" });
    });
}

function withSocketErrors(socket, fn) {
  try {
    fn();
  } catch {
    socket.destroy();
  }
}

function httpError(statusCode, message) {
  const err = new Error(message);
  err.statusCode = statusCode;
  return err;
}

if (require.main === module) {
  const port = Number(process.env.PORT || 8787);
  const server = createServer();
  server.listen(port, () => {
    console.log(`LylatLink signaling server listening on :${port}`);
  });
}

module.exports = {
  createServer,
  normalizePlayerCodes,
};
