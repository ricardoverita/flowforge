import assert from "node:assert/strict";
import test from "node:test";
import { ApiError, createApiClient } from "../lib/api-client";

test("workflow requests do not cache changing server state", async () => {
  let called = false;
  const fetcher: typeof fetch = async (url, options) => {
    assert.equal(url, "http://localhost:8080/api/v1/workflows");
    assert.equal(options?.cache, "no-store");
    assert.ok(options?.signal instanceof AbortSignal);
    called = true;
    return Response.json([{ id: "definition-1" }]);
  };
  const result = await createApiClient("http://localhost:8080/", fetcher).workflows();
  assert.equal(result[0]?.id, "definition-1");
  assert.ok(called);
});

test("execution requests forward idempotency and server-only bearer authentication", async () => {
  const fetcher: typeof fetch = async (url, options) => {
    assert.equal(url, "https://api.example.test/api/v1/workflows/a%2Fb/executions");
    assert.equal(options?.method, "POST");
    const headers = new Headers(options?.headers);
    assert.equal(headers.get("Idempotency-Key"), "customer-001");
    assert.equal(headers.get("Authorization"), "Bearer server-secret");
    assert.deepEqual(JSON.parse(String(options?.body)), { input: { customer_id: "001" } });
    return Response.json({ id: "execution-1" }, { status: 201 });
  };
  const result = await createApiClient("https://api.example.test", fetcher, "server-secret")
    .startExecution("a/b", { customer_id: "001" }, "customer-001");
  assert.equal(result.id, "execution-1");
});

test("upstream response bodies never become user-facing errors", async () => {
  const fetcher: typeof fetch = async () => new Response("private connection string", { status: 500 });
  await assert.rejects(createApiClient("http://localhost:8080", fetcher).workflows(), error => {
    assert.ok(error instanceof ApiError);
    assert.equal(error.status, 500);
    assert.ok(!error.message.includes("private"));
    return true;
  });
});

test("JSON editor input preserves large numeric tokens exactly", async () => {
  const fetcher: typeof fetch = async (_url, options) => {
    assert.equal(options?.body, '{"input":{"value":9007199254740993,"exponent":1e400}}');
    return Response.json({ id: "execution-1" }, { status: 201 });
  };
  await createApiClient("http://localhost:8080", fetcher)
    .startExecution("workflow-1", '{"value":9007199254740993,"exponent":1e400}');
});

test("not-found and conflict responses retain actionable status", async () => {
  for (const status of [404, 409]) {
    const fetcher: typeof fetch = async () => new Response("", { status });
    await assert.rejects(createApiClient("http://localhost:8080", fetcher).execution("unknown"),
      error => error instanceof ApiError && error.status === status);
  }
});

test("network failures and invalid JSON produce bounded public errors", async () => {
  const unavailable: typeof fetch = async () => { throw new Error("secret DNS context"); };
  await assert.rejects(createApiClient("http://localhost:8080", unavailable).workflows(),
    error => error instanceof ApiError && error.status === 503);
  const malformed: typeof fetch = async () => new Response("not-json");
  await assert.rejects(createApiClient("http://localhost:8080", malformed).workflows(),
    error => error instanceof ApiError && error.status === 502);
});

test("API URL rejects unsupported schemes and embedded secrets", () => {
  for (const url of ["file:///tmp/api", "http://user:secret@localhost:8080", "http://localhost:8080?secret=1"]) {
    assert.throws(() => createApiClient(url));
  }
});
