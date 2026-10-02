import type { Metadata } from "next";
import { api } from "@/lib/server-api";
import { ApiNotice } from "@/components/api-notice";
import { WorkflowForm } from "@/components/forms";
import { formatTime } from "@/components/status";
import type { Workflow } from "@/lib/types";

export const metadata: Metadata = { title: "Workflows" };
export const dynamic = "force-dynamic";

export default async function WorkflowsPage({ searchParams }: { searchParams: Promise<{ created?: string }> }) {
  const { created } = await searchParams;
  let workflows: Workflow[] = [];
  let failure: unknown;
  try { workflows = await api().workflows(); } catch (error) { failure = error; }
  return <><p className="eyebrow">Versioned definitions</p><h1>Workflows</h1>
    <p className="intro">Steps run in the order they appear in a definition. Existing versions stay immutable.</p>
    {created === "1" && <p className="notice success" role="status">Workflow definition created.</p>}
    {failure !== undefined && <ApiNotice error={failure} />}
    <section className="panel"><h2>Registered versions</h2>
      {!workflows.length ? <p className="muted">No definitions loaded. Create your first workflow below.</p> :
        <div className="table-wrap"><table><thead><tr><th>Name</th><th>Version</th><th>Steps</th><th>Created</th></tr></thead><tbody>
          {workflows.map(workflow => <tr key={workflow.id}><td><details><summary>{workflow.name}</summary>
            <p className="muted">{workflow.description}</p><code>{workflow.id}</code><ol>{workflow.steps.map(step => <li key={step.id}>{step.name} <code>{step.task_type}</code></li>)}</ol>
          </details></td><td>v{workflow.version}</td><td>{workflow.steps.length}</td><td>{formatTime(workflow.created_at)}</td></tr>)}
        </tbody></table></div>}
    </section>
    <section className="panel form-panel"><h2>Create a workflow version</h2><WorkflowForm /></section>
  </>;
}
