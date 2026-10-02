import type { Execution, NewWorkflow, Workflow, WorkflowEvent } from "./types";

const messages: Record<number, string> = {
  400: "The API rejected this request. Check the input fields.",
  401: "The API requires authentication. Check the console's server token configuration.",
  403: "The API denied this request.",
  404: "This resource was not found.",
  409: "This request conflicts with an existing workflow or idempotency key.",
  413: "This request is too large.",
  422: "The API rejected this request. Check the input fields.",
};

export class ApiError extends Error {
  constructor(readonly status: number) {
    super(messages[status] ?? "FlowForge API is unavailable. Please try again.");
    this.name = "ApiError";
  }
}

export function createApiClient(baseURL: string, fetcher: typeof fetch = fetch, token?: string) {
  const url = new URL(baseURL);
  if (!["http:", "https:"].includes(url.protocol) || url.username || url.password || url.search || url.hash) {
    throw new Error("FLOWFORGE_API_URL must be an HTTP(S) URL without credentials, query, or fragment.");
  }
  const base = url.toString().replace(/\/$/, "");

  async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
    let response: Response;
    try {
      response = await fetcher(`${base}${path}`, {
        ...init,
        headers: { Accept: "application/json", ...(token ? { Authorization: `Bearer ${token}` } : {}), ...init.headers },
        cache: "no-store",
        signal: AbortSignal.timeout(10_000),
      });
    } catch {
      throw new ApiError(503);
    }
    if (!response.ok) throw new ApiError(response.status);
    try {
      return await response.json() as T;
    } catch {
      throw new ApiError(502);
    }
  }

  return {
    workflows: () => request<Workflow[]>("/api/v1/workflows"),
    workflow: (id: string) => request<Workflow>(`/api/v1/workflows/${encodeURIComponent(id)}`),
    createWorkflow: (workflow: NewWorkflow) => request<Workflow>("/api/v1/workflows", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(workflow),
    }),
    startExecution: (workflowID: string, input: Record<string, unknown> | string, idempotencyKey?: string) =>
      request<Execution>(`/api/v1/workflows/${encodeURIComponent(workflowID)}/executions`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          ...(idempotencyKey ? { "Idempotency-Key": idempotencyKey } : {}),
        },
        // Preserve valid JSON number tokens from the editor instead of round-tripping through Number.
        body: typeof input === "string" ? `{"input":${input}}` : JSON.stringify({ input }),
      }),
    execution: (id: string) => request<Execution>(`/api/v1/executions/${encodeURIComponent(id)}`),
    events: (id: string) => request<WorkflowEvent[]>(`/api/v1/executions/${encodeURIComponent(id)}/events`),
  };
}
