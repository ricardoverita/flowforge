import { ApiError } from "@/lib/api-client";

export function ApiNotice({ error }: { error: unknown }) {
  return <div className="notice error" role="alert">
    <strong>{error instanceof ApiError ? error.message : "The console could not load API data."}</strong>
    <p>Check that the API is running and FLOWFORGE_API_URL points to it, then reload this page.</p>
  </div>;
}
