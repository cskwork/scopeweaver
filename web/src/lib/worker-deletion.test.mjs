import assert from "node:assert/strict";
import fs from "node:fs";
import { registerHooks } from "node:module";
import test from "node:test";
import vm from "node:vm";
import ts from "typescript";

// Exercise the actual row predicate without adding a component test framework.
const source = fs.readFileSync(new URL("../app/(main)/function/tasks/detail/_tabs/sessions-tab.tsx", import.meta.url), "utf8");
const ast = ts.createSourceFile("sessions-tab.tsx", source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
const sessionItem = ast.statements.find((node) => ts.isFunctionDeclaration(node) && node.name?.text === "SessionItem");
let predicate;
function visit(node) {
  if (ts.isVariableDeclaration(node) && node.name.getText(ast) === "cancellable") predicate = node.initializer.getText(ast);
  ts.forEachChild(node, visit);
}
assert.ok(sessionItem, "SessionItem exists");
visit(sessionItem);
assert.ok(predicate, "SessionItem defines its delete-action gate");
const cancellable = (s, deleted = false) => vm.runInNewContext(predicate, { s, deleted });

test("worker delete action accepts active and finished sessions, excluding inherited and deleted rows", () => {
  for (const status of ["running", "paused", "pending", "done", "blocked", "exhausted", "stopped"]) {
    const worker = { role: "worker", status };
    assert.equal(cancellable(worker), true, status);
    assert.equal(cancellable({ ...worker, inherited: true }), false, `${status}: inherited`);
    assert.equal(cancellable(worker, true), false, `${status}: deleted row`);
    for (const role of ["mainagent", "planner", "system"]) {
      assert.equal(cancellable({ ...worker, role }), false, `${status}: ${role}`);
    }
  }
  assert.equal(cancellable({ role: "worker", status: "deleted" }), false);
});

// Resolve the same aliases as Next.js while loading the real TypeScript mock client.
const hooks = registerHooks({
  resolve(specifier, context, nextResolve) {
    const url = specifier.startsWith("@/")
      ? new URL(`../${specifier.slice(2)}`, import.meta.url)
      : specifier.startsWith(".") ? new URL(specifier, context.parentURL) : null;
    if (url && !/\.[a-z]+$/i.test(url.pathname)) {
      for (const suffix of [".ts", "/index.ts"]) {
        const candidate = new URL(url.href + suffix);
        if (fs.existsSync(candidate)) return nextResolve(candidate.href, context);
      }
    }
    return nextResolve(specifier, context);
  },
});
const { mockHandle } = await import("./mock/handler.ts");
hooks.deregister();

test("demo deletion accepts every active and finished state in both modes and rejects inherited or deleted intents", async () => {
  const intents = await mockHandle("GET", "/exploration/intents");
  const worker = intents.find((intent) => intent.state === "done" && !intent.inherited);
  assert.ok(worker, "completed worker fixture exists");
  const original = { ...worker };
  try {
    for (const state of ["running", "paused", "open", "done", "blocked", "exhausted", "stopped"]) {
      for (const mode of ["soft", "hard"]) {
        Object.assign(worker, original, { state });
        const index = intents.indexOf(worker);
        const result = await mockHandle("POST", `/tasks/test/intents/${worker.id}/control`, JSON.stringify({ action: "cancel", reason: "Unreliable output", mode }));
        if (mode === "soft") {
          assert.equal(result.state, "deleted");
          assert.equal(worker.delete_reason, "Unreliable output");
          assert.equal(intents.includes(worker), true);
        } else {
          assert.equal(result.deleted.intents, 1);
          assert.equal(intents.includes(worker), false);
          intents.splice(index, 0, worker);
        }
      }
    }
    for (const [state, inherited] of [["done", true], ["deleted", false]]) {
      Object.assign(worker, original, { state, inherited });
      await assert.rejects(mockHandle("POST", `/tasks/test/intents/${worker.id}/control`, JSON.stringify({ action: "cancel", reason: "Unreliable output", mode: "hard" })));
    }
  } finally {
    Object.assign(worker, original);
    if (original.delete_reason === undefined) delete worker.delete_reason;
    if (original.inherited === undefined) delete worker.inherited;
  }
});
