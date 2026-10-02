import assert from "node:assert/strict";
import test from "node:test";
import { renderToStaticMarkup } from "react-dom/server";
import { formatTime, Status } from "../components/status";

test("status badges present lifecycle text with an accessible visible label", () => {
  const html = renderToStaticMarkup(<Status value="running" />);
  assert.match(html, /status-running/);
  assert.match(html, />running</);
});

test("timestamps render UTC consistently and tolerate absent values", () => {
  assert.equal(formatTime("2026-10-02T17:00:00-05:00"), "2026-10-02 22:00:00 UTC");
  assert.equal(formatTime(null), "—");
  assert.equal(formatTime("invalid"), "—");
});
