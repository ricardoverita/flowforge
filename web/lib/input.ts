import type { NewWorkflow } from "./types";

export const onboardingSteps: NewWorkflow["steps"] = [
  { name: "verify_identity", task_type: "identity.verify", max_attempts: 3, timeout_seconds: 30 },
  { name: "risk_check", task_type: "risk.check", max_attempts: 3, timeout_seconds: 30 },
  { name: "create_account", task_type: "account.create", max_attempts: 3, timeout_seconds: 30 },
  { name: "send_notification", task_type: "notification.send", max_attempts: 3, timeout_seconds: 30 },
];

export function parseObject(value: string): Record<string, unknown> {
  let input: unknown;
  try { input = JSON.parse(value); } catch { throw new Error("Input must be valid JSON."); }
  if (!input || typeof input !== "object" || Array.isArray(input)) {
    throw new Error("Input must be a JSON object.");
  }
  return input as Record<string, unknown>;
}

export function parseWorkflow(form: FormData): NewWorkflow {
  const name = String(form.get("name") ?? "").trim();
  const description = String(form.get("description") ?? "").trim();
  const version = Number(form.get("version"));
  if (!name || name.length > 100) throw new Error("A workflow name of up to 100 characters is required.");
  if (!Number.isSafeInteger(version) || version < 1 || version > 1_000_000) throw new Error("Version must be an integer between 1 and 1,000,000.");
  if (description.length > 2000) throw new Error("Description must be at most 2,000 characters.");
  let steps: unknown;
  try { steps = JSON.parse(String(form.get("steps") ?? "")); } catch { throw new Error("Steps must be valid JSON."); }
  if (!Array.isArray(steps) || steps.length === 0 || steps.length > 100) throw new Error("Define between 1 and 100 steps.");
  const names = new Set<string>();
  for (const step of steps) {
    if (!step || typeof step !== "object" || typeof step.name !== "string" || !step.name.trim() || step.name.length > 100
      || typeof step.task_type !== "string" || step.task_type.length > 128 || !/^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$/.test(step.task_type) || "id" in step) {
      throw new Error("Each step needs a name and task_type. IDs are assigned by the API.");
    }
    if (names.has(step.name)) throw new Error("Step names must be unique within the workflow.");
    names.add(step.name);
    if (!Number.isSafeInteger(step.max_attempts) || step.max_attempts < 1 || step.max_attempts > 10
      || !Number.isSafeInteger(step.timeout_seconds) || step.timeout_seconds < 1 || step.timeout_seconds > 3600) {
      throw new Error("Each step needs max_attempts (1–10) and timeout_seconds (1–3,600).");
    }
  }
  return { name, description, version, steps: steps as NewWorkflow["steps"] };
}

export function parseResourceID(value: FormDataEntryValue | null): string {
  if (typeof value !== "string" || !/^[a-zA-Z0-9_-]{1,128}$/.test(value)) throw new Error("Enter a valid resource ID.");
  return value;
}
