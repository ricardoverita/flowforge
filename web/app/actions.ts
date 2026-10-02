"use server";

import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";
import { api } from "@/lib/server-api";
import { parseObject, parseResourceID, parseWorkflow } from "@/lib/input";
import { ApiError } from "@/lib/api-client";

export interface ActionState { error: string }

function message(error: unknown): string {
  if (error instanceof ApiError || error instanceof Error) return error.message;
  return "The request could not be completed.";
}

export async function createWorkflowAction(_state: ActionState, form: FormData): Promise<ActionState> {
  try {
    await api().createWorkflow(parseWorkflow(form));
  } catch (error) {
    return { error: message(error) };
  }
  revalidatePath("/workflows");
  redirect("/workflows?created=1");
}

export async function startExecutionAction(_state: ActionState, form: FormData): Promise<ActionState> {
  let executionID: string;
  try {
    const workflowID = parseResourceID(form.get("workflow_id"));
    const input = String(form.get("input") ?? "{}");
    parseObject(input);
    const key = String(form.get("idempotency_key") ?? "").trim();
    if (key.length > 128 || /[^\x21-\x7e]/.test(key)) throw new Error("Idempotency key must contain at most 128 visible ASCII characters, without spaces.");
    const execution = await api().startExecution(workflowID, input, key || undefined);
    executionID = execution.id;
  } catch (error) {
    return { error: message(error) };
  }
  redirect(`/executions/${encodeURIComponent(executionID)}`);
}

export async function lookupExecutionAction(_state: ActionState, form: FormData): Promise<ActionState> {
  let executionID: string;
  try { executionID = parseResourceID(form.get("execution_id")); }
  catch (error) { return { error: message(error) }; }
  redirect(`/executions/${encodeURIComponent(executionID)}`);
}
