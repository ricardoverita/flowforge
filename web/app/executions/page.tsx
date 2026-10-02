import type { Metadata } from "next";
import { api } from "@/lib/server-api";
import { ApiNotice } from "@/components/api-notice";
import { ExecutionLookupForm, StartExecutionForm } from "@/components/forms";
import type { Workflow } from "@/lib/types";

export const metadata: Metadata = { title: "Executions" };
export const dynamic = "force-dynamic";

export default async function ExecutionsPage() {
  let workflows: Workflow[] = [];
  let failure: unknown;
  try { workflows = await api().workflows(); } catch (error) { failure = error; }
  return <><p className="eyebrow">Durable process instances</p><h1>Executions</h1>
    <p className="intro">Start a specific workflow version or look up an execution returned by the API.</p>
    {failure !== undefined && <ApiNotice error={failure} />}
    <div className="grid two-columns">
      <section className="panel"><h2>Start execution</h2>{workflows.length ? <StartExecutionForm workflows={workflows} /> : <p className="muted">Register a workflow before starting an execution.</p>}</section>
      <section className="panel"><h2>Inspect execution</h2><ExecutionLookupForm /><p className="muted">The initial API supports lookup by ID. A searchable execution list is planned.</p></section>
    </div>
  </>;
}
