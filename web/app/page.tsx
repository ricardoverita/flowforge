import Link from "next/link";
import { api } from "@/lib/server-api";
import { ApiNotice } from "@/components/api-notice";
import type { Workflow } from "@/lib/types";

export const dynamic = "force-dynamic";

export default async function Dashboard() {
  let workflows: Workflow[];
  try { workflows = await api().workflows(); } catch (error) {
    return <><h1>Dashboard</h1><ApiNotice error={error} /></>;
  }
  return <>
    <p className="eyebrow">Control plane</p><h1>Workflow orchestration</h1>
    <p className="intro">Define a process, start an execution, and follow each step through its durable history.</p>
    <div className="grid cards">
      <section className="card"><p className="muted">Workflow versions</p><p className="metric">{workflows.length}</p><Link href="/workflows">Manage definitions →</Link></section>
      <section className="card"><h2>Run a workflow</h2><p>Start a registered version with a JSON input, or inspect an execution by its ID.</p><Link href="/executions">Open executions →</Link></section>
      <section className="card"><h2>Durable by design</h2><p>PostgreSQL records execution state. JetStream distributes work to workers. Repeated messages are handled idempotently.</p></section>
    </div>
    <section className="panel"><h2>First workflow</h2><p>The example worker simulates customer onboarding. Each task runs outside the engine.</p>
      <div className="sequence" aria-label="Customer onboarding steps"><span>Verify identity</span><span aria-hidden="true">→</span><span>Risk check</span><span aria-hidden="true">→</span><span>Create account</span><span aria-hidden="true">→</span><span>Send notification</span></div>
      <p className="muted">This console provides definitions, execution creation, and history lookup. Execution listing and filtering are planned.</p>
    </section>
  </>;
}
