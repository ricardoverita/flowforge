import assert from "node:assert/strict";
import test from "node:test";
import { onboardingSteps, parseObject, parseResourceID, parseWorkflow } from "../lib/input";

function workflowForm(steps: unknown = onboardingSteps): FormData {
  const form = new FormData();
  form.set("name", "customer-onboarding");
  form.set("version", "1");
  form.set("description", "Simulated example");
  form.set("steps", JSON.stringify(steps));
  return form;
}

test("JSON input accepts objects and rejects malformed or non-object values", () => {
  assert.deepEqual(parseObject('{"customer_id":"001"}'), { customer_id: "001" });
  for (const value of ["null", "[]", "42", "false", "{broken"]) assert.throws(() => parseObject(value));
});

test("workflow form preserves explicit immutable version and ordered task policies", () => {
  const workflow = parseWorkflow(workflowForm());
  assert.equal(workflow.version, 1);
  assert.deepEqual(workflow.steps.map(step => step.task_type), ["identity.verify", "risk.check", "account.create", "notification.send"]);
  assert.ok(workflow.steps.every(step => step.max_attempts === 3 && step.timeout_seconds === 30));
});

test("workflow form rejects duplicates and policies outside API limits", () => {
  const step = onboardingSteps[0]!;
  for (const steps of [[], [step, step], [{ ...step, max_attempts: 11 }], [{ ...step, timeout_seconds: 3601 }], [{ ...step, task_type: "identity.*" }], [{ ...step, id: "user-defined" }]]) {
    assert.throws(() => parseWorkflow(workflowForm(steps)));
  }
});

test("resource IDs cannot change the lookup route", () => {
  assert.equal(parseResourceID("550e8400-e29b-41d4-a716-446655440000"), "550e8400-e29b-41d4-a716-446655440000");
  for (const value of [null, "../workflows", "x?secret=1", "", "a/b"]) assert.throws(() => parseResourceID(value));
});
