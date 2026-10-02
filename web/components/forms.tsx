"use client";

import { useActionState } from "react";
import { useFormStatus } from "react-dom";
import { createWorkflowAction, lookupExecutionAction, startExecutionAction } from "@/app/actions";
import { onboardingSteps } from "@/lib/input";
import type { Workflow } from "@/lib/types";

function Submit({ children }: { children: React.ReactNode }) {
  const { pending } = useFormStatus();
  return <button type="submit" disabled={pending}>{pending ? "Sending…" : children}</button>;
}

function FormError({ error }: { error: string }) {
  return error ? <p className="notice error" role="alert">{error}</p> : null;
}

export function WorkflowForm() {
  const [state, action] = useActionState(createWorkflowAction, { error: "" });
  return <form action={action} className="stack">
    <FormError error={state.error} />
    <div className="form-row">
      <label>Name<input name="name" defaultValue="customer-onboarding" required maxLength={100} /></label>
      <label>Version<input name="version" type="number" defaultValue={1} min={1} max={1_000_000} step={1} required /></label>
    </div>
    <label>Description<input name="description" defaultValue="A simulated customer onboarding workflow." maxLength={2000} /></label>
    <label>Sequential steps<textarea name="steps" defaultValue={JSON.stringify(onboardingSteps, null, 2)} rows={17} required spellCheck={false} /></label>
    <p className="muted">Definitions are immutable. Use a new version to change a workflow.</p>
    <Submit>Create definition</Submit>
  </form>;
}

export function StartExecutionForm({ workflows }: { workflows: Workflow[] }) {
  const [state, action] = useActionState(startExecutionAction, { error: "" });
  return <form action={action} className="stack">
    <FormError error={state.error} />
    <label>Workflow version<select name="workflow_id" required defaultValue={workflows[0]?.id}>
      {workflows.map(workflow => <option key={workflow.id} value={workflow.id}>{workflow.name} · v{workflow.version}</option>)}
    </select></label>
    <label>Input (JSON object)<textarea name="input" defaultValue={'{ "customer_id": "demo-customer" }'} rows={4} required spellCheck={false} /></label>
    <label>Idempotency key (optional)<input name="idempotency_key" maxLength={128} placeholder="onboarding-demo-001" /></label>
    <p className="muted">Reuse a key with the same workflow and input to retrieve the same execution.</p>
    <Submit>Start execution</Submit>
  </form>;
}

export function ExecutionLookupForm() {
  const [state, action] = useActionState(lookupExecutionAction, { error: "" });
  return <form action={action} className="stack">
    <FormError error={state.error} />
    <label>Execution ID<input name="execution_id" required maxLength={128} placeholder="Paste the execution ID returned by the API" /></label>
    <Submit>Open execution</Submit>
  </form>;
}
