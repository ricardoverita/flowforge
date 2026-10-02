import type { Metadata } from "next";
import Link from "next/link";
import { api } from "@/lib/server-api";
import { ApiNotice } from "@/components/api-notice";
import { Status, formatTime } from "@/components/status";
import { Refresh } from "@/components/refresh";
import type { Execution, WorkflowEvent } from "@/lib/types";

export const metadata: Metadata = { title: "Execution detail" };
export const dynamic = "force-dynamic";

export default async function ExecutionPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  let execution: Execution;
  let events: WorkflowEvent[];
  try { [execution, events] = await Promise.all([api().execution(id), api().events(id)]); }
  catch (error) { return <><h1>Execution detail</h1><ApiNotice error={error} /><Link href="/executions">Back to executions</Link></>; }
  return <>
    <div className="page-heading"><div><p className="eyebrow">Execution detail</p><h1><Status value={execution.status} /></h1></div><Refresh /></div>
    <p className="execution-id"><code>{execution.id}</code></p>
    <section className="panel"><dl className="metadata">
      <div><dt>Workflow version</dt><dd><code>{execution.workflow_id}</code> · v{execution.workflow_version}</dd></div>
      <div><dt>Created</dt><dd>{formatTime(execution.created_at)}</dd></div>
      <div><dt>Started</dt><dd>{formatTime(execution.started_at)}</dd></div>
      <div><dt>Completed</dt><dd>{formatTime(execution.completed_at)}</dd></div>
      <div><dt>Correlation ID</dt><dd><code>{execution.correlation_id}</code></dd></div>
      <div><dt>Revision</dt><dd>{execution.revision}</dd></div>
    </dl></section>
    <section className="panel"><h2>Steps</h2><div className="table-wrap"><table>
      <thead><tr><th>Step</th><th>Status</th><th>Attempt</th><th>Started</th><th>Next retry / deadline</th></tr></thead>
      <tbody>{execution.steps.map(step => <tr key={step.id}><td><strong>{step.name}</strong><br /><code>{step.task_type}</code>{step.error_code && <p className="error-text">{step.error_code}</p>}</td><td><Status value={step.status} /></td><td>{step.attempt} / {step.max_attempts}</td><td>{formatTime(step.started_at)}</td><td>{formatTime(step.retry_at ?? step.deadline_at)}</td></tr>)}</tbody>
    </table></div></section>
    <section className="panel"><h2>Execution history</h2><ol className="timeline">
      {events.map(event => <li key={event.id}><time>{formatTime(event.occurred_at)}</time><div><strong>{event.type}</strong>{event.step_id && <p><code>{event.step_id}</code>{event.attempt ? ` · attempt ${event.attempt}` : ""}</p>}<details><summary>Event data</summary><pre>{JSON.stringify(event.data, null, 2)}</pre></details></div></li>)}
    </ol>{!events.length && <p className="muted">No events recorded yet.</p>}</section>
    <section className="panel"><details><summary>Execution input and step output</summary><h3>Input</h3><pre>{JSON.stringify(execution.input, null, 2)}</pre>
      {execution.steps.filter(step => step.output).map(step => <div key={step.id}><h3>{step.name}</h3><pre>{JSON.stringify(step.output, null, 2)}</pre></div>)}
    </details></section>
  </>;
}
