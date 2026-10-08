# Agent Runtime Unification Report

Date: 2026-10-08

## Summary

This change implements a shared, durable tool execution path for rule-derived replenishment work, routes the legacy chat inventory lookup through that executor, adds a bounded chat entry for background replenishment requests, and makes real task steps visible/cancellable in AIManager. It closes an important portion of Phase 2, but **does not complete the full requested phase**. In particular, the background replenishment workflow currently uses deterministic rule evidence and registered tools; it does not invoke an LLM planner. Persistent user-defined schedules and their management UI/API are not implemented.

## Old and new execution architecture

Before this change, chat tools ran in `controllers.agentExecuteTool`, while Monitor independently leased tasks and directly created drafts. Chat tasks were persisted after the synchronous LLM turn and did not have durable step rows.

The new `agentnative.RuntimeExecutor` provides a Gin-independent `ToolExecutor` interface with `ExecutionContext`, `ToolInput`, and `ToolOutput`. Both the Monitor task worker and the existing chat inventory lookup can call this executor. The existing LLM provider selection and legacy tools remain available; the full legacy tool registry and all of its business operations have not yet been migrated to the new registry.

The registered tools currently are:

- `retail.product_inventory`: validated product inventory search through `RetailReadService`, used by the old chat inventory tool.
- `retail.replenishment_context`: current stock and valid-order sales evidence through `RetailReadService`.
- `monitor.get_signal`: reads a persisted rule signal within the fixed single-tenant scope.
- `replenishment.create_internal_draft`: creates or returns the internal replenishment draft by task ID; never creates a purchase order or mutates stock.

`RuntimeExecutor.Execute` centralizes principal/tenant checks, permission checks for the draft write, timeout/cancellation, errors, and `AgentToolCallAudit` writes. The inventory tool remains read-only. The currently migrated tool surface does not include the general sales analytics, arbitrary restricted dataset queries, or every legacy tool.

## Durable steps, leases, and recovery

`AgentWorkTaskStep` persists each workflow tool call with sequence, type, tool name, input summary, status, timestamps, result, retry count, error, and a unique idempotency key. `AgentToolCallAudit` records each executor invocation, its principal, task/call IDs, input summary, outcome, and timing.

Monitor tasks are claimed with a conditional update and a lease. `AGENT_RUNTIME_ENABLED=1` starts a worker that processes pending tasks on startup and polls every five seconds, so expired leases and queued tasks can resume after restart. When this flag is enabled, it takes precedence over the legacy `AGENT_MONITOR_ENABLED` worker to avoid two monitor tickers. The daily scan itself still uses a 24-hour process interval.

The internal draft has a unique `TaskID`. If the process dies after its draft transaction commits but before the last step row is marked successful, the next worker pass reconciles the completed task and closes the unfinished draft step from the persisted draft/task state. The three workflow calls are safe to retry: two are reads and the write uses task-scoped uniqueness. Cancellation is persisted and checked between tool calls; a tool already in progress can finish before cancellation takes effect.

The chat entry recognizes a narrow set of replenishment-analysis phrases. It scans and persists the signal/task before returning an accepted response with the task ID; the worker then processes it in the background. Ordinary chat remains synchronous. This entry links to an existing open signal task where possible. It is not a general-purpose durable execution planner, and its workflow does not call an LLM.

## Monitor integration and evidence

Rule scanning remains deterministic and continues to own low-stock detection/deduplication. Its persisted tasks now use the same RuntimeExecutor and create-draft tool path as the chat inventory tool. Evidence comes from persisted signal metrics plus a fresh `RetailReadService` read. The draft explicitly states the lack of supplier, lead-time, in-transit order, and budget data; it does not invent any of them.

The existing human review workflow remains intact. The runtime has not added semantic LLM analysis, model-call budget controls, cost limits, or content-hash based repeat-analysis suppression beyond the existing signal fingerprint/open-signal deduplication.

## API and workbench

Added admin-only APIs:

- `GET /api/ai/manager/tasks/:id/steps`
- `POST /api/ai/manager/tasks/:id/cancel`

AIManager now polls while tasks are active, shows persisted tool steps and results/errors, and offers cancellation. Chat messages for the narrow background replenishment entry carry the task ID and continue to link to the manager. Swagger docs were regenerated and the route-contract test passes.

## Persistent schedules

**Not implemented.** There is no database schedule definition, timezone-aware next-run calculation, schedule CRUD/pause/resume/cancel API, run history, natural-language schedule parser, or multi-worker schedule claim. `AGENT_RUNTIME_ENABLED` replaces the old monitor worker but the monitor interval remains a 24-hour ticker. Do not advertise user-created recurring schedules as available.

## Validation results

All database validation ran against a fresh temporary MySQL 8 container with no mounted data volume, using separate `erp_agent_native_test` and `erp_audit_test` databases. The test container was stopped and removed afterward.

- `go test ./...`: passed with `ERP_AUDIT_DB=1` and the isolated MySQL DSN.
- `go vet ./...`: passed.
- `npm run build`: passed; Vite emitted the existing large-chunk and stale Browserslist warnings.
- `node --test src/**/*.test.mjs`: 31 passed, 0 failed.
- `go generate .`: completed; swag emitted a Go runtime constant-evaluation warning but generated docs successfully.
- `git diff --check`: passed.
- The isolated Monitor integration test verified three actual successful persisted steps and three tool audit records, one unique replenishment draft, stale-lease recovery, and no purchase-order or inventory mutation. It also simulated the crash window after the draft transaction and verified step reconciliation.

**Not verified:** forced process termination of a live Worker at step two/three and restart as a separate process; browser-level chat-to-background-task interaction; actual LLM provider execution for background replenishment; scheduler behavior (not implemented); mock/real provider cost and failure fallback (background workflow currently makes no model calls).

## Remaining work and risks

1. Add an LLM planner behind a bounded provider interface for signal analysis, with deterministic fallback, evidence persistence, call-count/cost budgets, and tests using a controlled Mock Provider.
2. Migrate the remaining approved chat tools—especially general sales analysis and restricted data query—to the shared registry/executor and enforce one schema and authorization contract end to end.
3. Implement persistent schedule definitions and run records, timezone and trigger-time validation, multi-worker atomic claiming, natural-language create/pause/resume parsing, API/UI controls, and the runtime feature flag migration from ticker to persisted scheduling.
4. Add a controlled crash/restart integration harness that kills a worker between individual steps, plus browser verification of chat links, progress, errors, and cancellation.
5. Cancellation is cooperative between tools. An operation already executing is not interrupted by the status update unless its context also receives cancellation; the current tool execution context is bounded by timeout but is not yet wired to a per-task cancellation channel.
6. `AGENT_RUNTIME_ENABLED` must be explicitly enabled in deployment for startup recovery and polling. The schema uses the current fixed `single` tenant scope; this is not multi-tenant support.

The requested full-phase acceptance criteria are therefore **partially met, not passed**. The recoverable deterministic replenishment workflow is the implemented priority slice; persistent scheduling and LLM-driven semantic analysis remain open.
