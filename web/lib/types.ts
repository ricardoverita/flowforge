export type ExecutionStatus = "pending" | "running" | "completed" | "failed" | "waiting" | "cancelled";
export type StepStatus = "pending" | "running" | "retry_wait" | "completed" | "failed" | "cancelled";

export interface StepDefinition {
  id: string;
  name: string;
  task_type: string;
  max_attempts: number;
  timeout_seconds: number;
}

export interface Workflow {
  id: string;
  name: string;
  version: number;
  description: string;
  steps: StepDefinition[];
  created_at: string;
}

export interface NewWorkflow {
  name: string;
  version: number;
  description: string;
  steps: Omit<StepDefinition, "id">[];
}

export interface StepExecution extends StepDefinition {
  position: number;
  status: StepStatus;
  attempt: number;
  input: unknown;
  output?: unknown;
  error_code?: string;
  started_at?: string | null;
  completed_at?: string | null;
  retry_at?: string | null;
  deadline_at?: string | null;
}

export interface Execution {
  id: string;
  workflow_id: string;
  workflow_version: number;
  status: ExecutionStatus;
  input: Record<string, unknown>;
  correlation_id: string;
  revision: number;
  created_at: string;
  started_at?: string | null;
  completed_at?: string | null;
  steps: StepExecution[];
}

export interface WorkflowEvent {
  id: number;
  execution_id: string;
  type: string;
  step_id?: string;
  attempt?: number;
  occurred_at: string;
  data: Record<string, unknown>;
}
