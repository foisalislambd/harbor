import assert from "node:assert/strict";
import test from "node:test";
import { messageFromFailedResponse } from "./harbor-api.ts";

const cloudflare = `<!DOCTYPE html> <!--[if lt IE 7]> <html class="no-js ie6 oldie" lang="en-US">`;

test("a proxy HTML page becomes a status message", () => {
  assert.equal(messageFromFailedResponse(524, cloudflare), "Proxy returned 524 timeout");
  assert.equal(messageFromFailedResponse(502, "<html><body>bad gateway</body></html>"), "Proxy returned 502 bad gateway");
  assert.equal(messageFromFailedResponse(504, "gateway timeout"), "Proxy returned 504 gateway timeout");
  assert.equal(messageFromFailedResponse(500, "nope"), "Proxy returned 500");
  assert.equal(messageFromFailedResponse(400, "nope"), "Something went wrong. Try again.");
  assert.equal(messageFromFailedResponse(404, "404 page not found"), "That could not be found. Refresh the page and try again.");
});

test("a JSON error is shown as Harbor wrote it", () => {
  assert.equal(
    messageFromFailedResponse(502, JSON.stringify({ error: "Telegram rejected that phone number (PHONE_NUMBER_INVALID). Use the international form, such as +15551234567" })),
    "Telegram rejected that phone number (PHONE_NUMBER_INVALID). Use the international form, such as +15551234567",
  );
});

