import "server-only";
import { createApiClient } from "./api-client";

export function api() {
  return createApiClient(process.env.FLOWFORGE_API_URL ?? "http://localhost:8080", fetch, process.env.FLOWFORGE_API_TOKEN);
}
