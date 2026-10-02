package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/Autumn-27/artex/locale"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/norma/permission"
	actool "github.com/Autumn-27/norma/tool"
)

// jsonResult marshals v to a JSON tool result.
func jsonResult(v any) (actool.Result, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return actool.Errorf(err.Error()), nil
	}
	return actool.Text(string(b)), nil
}

// 本文件实现 P2「跨任务编排工具集」(docs/跑分编排 §2 P2)。这些是 host 工具——需要
// 访问 Manager(任意任务的 Store)、Engine(暂停)、以及建任务流程,所以住在 server 层。
// 读类工具把「现有 per-task 工具」重定向到目标任务的 store 上跑(建一个临时 ToolSet
// 并 Call 其对应工具),从而复用完全相同的逻辑;控制类(spawn/pause)直接调 Manager/Engine。
// 它们像流量工具一样 seed 进 tools 表、按 agent 绑定(只绑给编排 agent 才可见)。

// hostTools is the runtime host-tool provider fed to ToolAugment: traffic tools
// (gated by capture) + cross-task orchestration tools + user-defined custom tools.
// The second return is the names of custom tools flagged `deferred` (schema
// withheld, routed via SearchExtraTools/ExecuteExtraTool). Per-agent binding still
// decides who actually sees any of them.
//
//nolint:unused // used as the hostTools provider in wireAgentAugment
func (s *Server) hostTools(ctx context.Context) ([]actool.CoreTool, map[string][]string) {
	tools := append(s.m.HostTools(locale.FromContext(ctx)), s.orchestrationTools(locale.FromContext(ctx))...)
	tools = append(tools, s.findingRetestTools(locale.FromContext(ctx))...)
	tools = append(tools, s.platformTools(locale.FromContext(ctx))...) // 平台操作工具(建改 skill/工具/MCP，给 Auto 用)
	custom, err := s.customToolsForLanguage(locale.FromContext(ctx))
	if err != nil {
		log.Printf(locale.Text(locale.ServerDefault(), "[custom-tool] Loading failed: %v"), err)
		return tools, nil
	}
	tools = append(tools, custom...)
	// deferred custom tools → name -> its bound agent keys. ToolAugment turns a
	// name into a deferred entry only for agents it's actually bound to (so we don't
	// advertise a tool the per-agent binding will drop from the callable set).
	deferred := map[string][]string{}
	rows, _ := s.m.pg.ListCustomTools()
	for _, t := range rows {
		if t.Deferred && t.Enabled {
			deferred[t.Key] = t.Agents
		}
	}
	return tools, deferred
}

// orchestrationTools returns the cross-task tool set. Bound per-agent via the
// tools table (default: no binding — opt-in for orchestration agents).
func (s *Server) orchestrationTools(langs ...locale.Lang) []actool.CoreTool {
	return []actool.CoreTool{
		s.toolListTasks(langs...),
		s.toolListLLMProfiles(langs...),
		s.toolSpawnTask(langs...),
		s.toolPauseTask(langs...),
		s.toolGetTaskGraph(langs...),
		s.toolListTaskFindings(langs...),
		s.toolAddHint(langs...),
		s.toolGetWorkerTrace(langs...),
		s.toolListWorkerTraces(langs...),
		s.toolSearchWorkerTraces(langs...),
		s.toolGetTaskNodeDetail(langs...),
		s.toolUpdateFindingReport(langs...),
		s.toolGetFindingTraffic(langs...),
		s.toolBindFindingTraffic(langs...),
	}
}

// --- schema helpers ---

func strParam(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

// parseProfileID reads an LLM profile id from a tool arg that may arrive as a JSON
// number (5) or a numeric string ("5"); returns 0 when absent/unparseable.
func parseProfileID(raw json.RawMessage) int64 {
	if len(raw) == 0 {
		return 0
	}
	var n int64
	if json.Unmarshal(raw, &n) == nil {
		return n
	}
	var str string
	if json.Unmarshal(raw, &str) == nil {
		v, _ := strconv.ParseInt(strings.TrimSpace(str), 10, 64)
		return v
	}
	return 0
}

func objSchema(props map[string]any, required ...string) map[string]any {
	m := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		req := make([]any, len(required))
		for i, r := range required {
			req[i] = r
		}
		m["required"] = req
	}
	return m
}

func roTool(name, desc string, schema map[string]any, run func(context.Context, json.RawMessage) (actool.Result, error)) actool.CoreTool {
	return actool.Build(actool.Spec{
		Name: name, Description: desc, Schema: schema,
		ReadOnly:   func(json.RawMessage) bool { return true },
		Concurrent: func(json.RawMessage) bool { return true },
		Permissions: func(context.Context, json.RawMessage, permission.Context) permission.Decision {
			return permission.Allowed()
		},
		Run: func(ctx context.Context, in json.RawMessage, _ *actool.ToolContext) (actool.Result, error) {
			return run(ctx, in)
		},
	})
}

func wrTool(name, desc string, schema map[string]any, run func(context.Context, json.RawMessage) (actool.Result, error)) actool.CoreTool {
	return actool.Build(actool.Spec{
		Name: name, Description: desc, Schema: schema,
		Permissions: func(context.Context, json.RawMessage, permission.Context) permission.Decision {
			return permission.Allowed()
		},
		Run: func(ctx context.Context, in json.RawMessage, _ *actool.ToolContext) (actool.Result, error) {
			return run(ctx, in)
		},
	})
}

// delegateToTask resolves the `task_id` in the input, builds a ToolSet bound to
// that task's store, strips task_id, and calls the chosen per-task tool — so the
// cross-task read reuses the exact in-task logic against another task.
func (s *Server) delegateToTask(ctx context.Context, in json.RawMessage, pick func(*agent.ToolSet) actool.CoreTool) (actool.Result, error) {
	var head struct {
		TaskID string `json:"task_id"`
	}
	_ = json.Unmarshal(in, &head)
	if strings.TrimSpace(head.TaskID) == "" {
		return actool.Errorf(locale.Text(locale.FromContext(ctx), "task_id is required")), nil
	}
	t, ok := s.m.Task(head.TaskID)
	if !ok {
		return actool.Errorf(locale.Text(locale.FromContext(ctx), "Task not found: ") + head.TaskID), nil
	}
	var m map[string]json.RawMessage
	_ = json.Unmarshal(in, &m)
	delete(m, "task_id")
	inner, _ := json.Marshal(m)
	tsx := agent.NewToolSet(t.Store, "orchestrator", locale.FromContext(ctx))
	if s.m.Assets() != nil {
		tsx.SetAssetStore(s.m.Assets(), s.m.Assets().Companies())
	}
	tsx.SetNotify(t.Notify)         // 通用唤醒（无专用回调的写操作走它；读工具为 no-op）
	tsx.SetNotifyHint(t.NotifyHint) // add_hint → 记一条「人新增了 N 条战略提示：…」触发并唤醒 planner
	return pick(tsx).Call(ctx, inner, nil)
}

// --- tools ---

func (s *Server) toolListTasks(langs ...locale.Lang) actool.CoreTool {
	return roTool("list_tasks",
		locale.Text(locale.First(langs), "List all tasks with id/description/objective/state/elapsed time/parent/LLM profile, so orchestration can inspect progress, stalls, and model assignments. Elapsed seconds run from creation to now for active tasks or last activity for terminal tasks. llm_profile is the planner/worker profile name; active profile means following the global selection."),
		objSchema(map[string]any{}),
		func(context.Context, json.RawMessage) (actool.Result, error) {
			lastAct, _ := s.m.PG().LastActivityAll()
			// id -> name to resolve each task's pinned LLM profile.
			profName := map[int64]string{}
			if profs, err := s.m.pg.ListProfiles(); err == nil {
				for _, p := range profs {
					profName[p.ID] = p.Name
				}
			}
			out := make([]map[string]any, 0)
			for _, t := range s.m.List() {
				status := s.deriveTaskStatus(t)
				end := lastAct[t.ExpID]
				if live := s.engine.LastActivity(t.ID); live > end {
					end = live
				}
				dur := int64(0)
				if status == "running" {
					dur = time.Now().Unix() - t.CreatedAt
				} else if end > t.CreatedAt {
					dur = end - t.CreatedAt
				}
				row := map[string]any{"id": t.ID, "description": t.Description, "goal": t.Goal, "status": status, "run_seconds": dur}
				if t.ParentRef != "" {
					row["parent_ref"] = t.ParentRef
				}
				llmState := t.llmStateSnapshot()
				if llmState.ProfileID == nil {
					row["llm_profile"] = locale.Text(locale.First(langs), "(active profile)")
				} else if n, ok := profName[*llmState.ProfileID]; ok {
					row["llm_profile"] = n
				} else {
					row["llm_profile"] = fmt.Sprintf(locale.Text(locale.First(langs), "#%d (deleted)"), *llmState.ProfileID)
				}
				out = append(out, row)
			}
			return jsonResult(out)
		})
}

// toolListLLMProfiles lists the available LLM profiles (name/model/active) so an
// orchestration agent can pick one for spawn_task's llm_profile. Never leaks keys.
func (s *Server) toolListLLMProfiles(langs ...locale.Lang) actool.CoreTool {
	return roTool("list_llm_profiles",
		locale.Text(locale.First(langs), "List available LLM profiles: id, name, model, format, and active status. Use an ID as spawn_task.llm_profile_id to assign a child task's model. API keys are never included."),
		objSchema(map[string]any{}),
		func(context.Context, json.RawMessage) (actool.Result, error) {
			profs, err := s.m.pg.ListProfiles()
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			out := make([]map[string]any, 0, len(profs))
			for _, p := range profs {
				out = append(out, map[string]any{
					"id": p.ID, "name": p.Name, "model": p.Model, "format": p.Format, "is_active": p.IsDefault,
				})
			}
			return jsonResult(map[string]any{"profiles": out})
		})
}

func (s *Server) toolSpawnTask(langs ...locale.Lang) actool.CoreTool {
	return wrTool("spawn_task",
		locale.Text(locale.First(langs), "Create a child task and start its exploration engine, returning task_id. Delegate one objective as an independent task. Optional parent_ref associates it with the current parent task."),
		objSchema(map[string]any{
			"description":            strParam(locale.Text(locale.First(langs), "Task description, a short title")),
			"goal":                   strParam(locale.Text(locale.First(langs), "Task objective: the outcome to achieve")),
			"parent_ref":             strParam(locale.Text(locale.First(langs), "Optional parent task ID for parent-child association")),
			"source_task_ids":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": fmt.Sprintf(locale.Text(locale.First(langs), "Optional read-only source task IDs, at most %d. The child may reference their established assets/conclusions as starting context. Unlike parent_ref, this is content inheritance."), db.MaxTaskSourceCount)},
			"llm_profile_id":         map[string]any{"type": "integer", "description": locale.Text(locale.First(langs), "Optional LLM profile ID for this child's planner/worker, from list_llm_profiles; otherwise inherit the parent's profile, then the globally active profile")},
			"timeout_seconds":        map[string]any{"type": "integer", "description": locale.Text(locale.First(langs), "Optional task timeout in seconds; triggers graceful settlement and terminal timeout state. Omitted/0 means unlimited.")},
			"plan_heartbeat_seconds": map[string]any{"type": "integer", "description": locale.Text(locale.First(langs), "Optional planner heartbeat in seconds: if no trigger arrives since the last planning round/task start, wake planning to avoid stalls and supervise workers. Omitted/0 uses the configured default.")},
			"seed_first_intent":      map[string]any{"type": "boolean", "description": locale.Text(locale.First(langs), "Optional for simple tasks: create an initial intent from description+goal so a worker starts without waiting for the first planner round. Default false uses the normal plan-then-execute flow.")},
		}, "description", "goal"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				Description          string          `json:"description"`
				Goal                 string          `json:"goal"`
				ParentRef            string          `json:"parent_ref"`
				SourceTaskIDs        []string        `json:"source_task_ids"`
				LLMProfileID         json.RawMessage `json:"llm_profile_id"`
				TimeoutSeconds       int             `json:"timeout_seconds"`
				PlanHeartbeatSeconds int             `json:"plan_heartbeat_seconds"`
				SeedFirstIntent      bool            `json:"seed_first_intent"`
			}
			_ = json.Unmarshal(in, &a)
			if strings.TrimSpace(a.Description) == "" {
				a.Description = locale.Text(locale.First(langs), "Untitled task")
			}
			if strings.TrimSpace(a.Goal) == "" {
				return actool.Errorf(locale.Text(locale.First(langs), "goal is required")), nil
			}
			if a.TimeoutSeconds < 0 {
				a.TimeoutSeconds = 0
			}
			// 只读继承来源任务：数量上限 + 每个 id 有效/去重/存在，校验规则与 HTTP 建任务一致。
			if len(a.SourceTaskIDs) > db.MaxTaskSourceCount {
				return actool.Errorf(fmt.Sprintf(locale.Text(locale.First(langs), "At most %d source tasks may be selected"), db.MaxTaskSourceCount)), nil
			}
			sourceIDs := make([]int64, 0, len(a.SourceTaskIDs))
			seenSources := map[int64]bool{}
			for _, raw := range a.SourceTaskIDs {
				id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
				if err != nil || id <= 0 || seenSources[id] {
					return actool.Errorf(locale.Text(locale.First(langs), "Source task ID is invalid or duplicated")), nil
				}
				if _, ok := s.m.Task(strconv.FormatInt(id, 10)); !ok {
					return actool.Errorf(fmt.Sprintf(locale.Text(locale.First(langs), "Source task #%d does not exist"), id)), nil
				}
				seenSources[id] = true
				sourceIDs = append(sourceIDs, id)
			}
			// LLM profile resolution: explicit id > inherit parent's pin > active(nil).
			var pin *int64
			if id := parseProfileID(a.LLMProfileID); id > 0 {
				if _, ok := s.loadProfileConfig(id); !ok {
					return actool.Errorf(fmt.Sprintf(locale.Text(locale.First(langs), "LLM profile #%d does not exist or has no API key"), id)), nil
				}
				pin = &id
			} else if a.ParentRef != "" {
				if pt, ok := s.m.Task(a.ParentRef); ok {
					pin = pt.LLMProfileID
				}
			}
			var llmIDs []int64
			if pin != nil {
				llmIDs = []int64{*pin}
			}
			t, err := s.m.CreateTaskWithOptions(a.Description, a.Goal, db.TaskCreateOptions{
				Language:             s.childTaskLanguage(ctx, a.ParentRef),
				SourceTaskIDs:        sourceIDs,
				LLMProfileIDs:        llmIDs,
				TimeoutSeconds:       a.TimeoutSeconds,
				PlanHeartbeatSeconds: a.PlanHeartbeatSeconds,
			})
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			if a.ParentRef != "" {
				t.ParentRef = a.ParentRef
				if id, e := strconv.ParseInt(t.ID, 10, 64); e == nil {
					_ = s.m.PG().SetParentRef(id, a.ParentRef)
				}
			}
			// 共享的建后流程,与 HTTP 建任务(server.go createTask)复用同一段 launchTask:
			// seed + 后台可见地做目标分解(第0轮/LLM步骤/逐条goal) + engine.Run。
			// seed_first_intent 默认 false(标准先规划再执行);简单任务可开启直接下发一 work 测试。
			s.launchTask(t, a.Description+" "+a.Goal, a.SeedFirstIntent)
			return actool.Text(fmt.Sprintf("task created: %s", t.ID)), nil
		})
}

func (s *Server) toolPauseTask(langs ...locale.Lang) actool.CoreTool {
	return wrTool("pause_task", locale.Text(locale.First(langs), "Pause a task and stop its planner/worker loops."),
		objSchema(map[string]any{"task_id": strParam(locale.Text(locale.First(langs), "Task ID to pause"))}, "task_id"),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				TaskID string `json:"task_id"`
			}
			_ = json.Unmarshal(in, &a)
			t, ok := s.m.Task(a.TaskID)
			if !ok {
				return actool.Errorf(locale.Text(locale.First(langs), "Task not found: ") + a.TaskID), nil
			}
			if _, err := s.applyTaskControlWithCause(t, "pause", agent.AbortPausedByOrchestrator); err != nil {
				return actool.Errorf(err.Error()), nil
			}
			return actool.Text("task paused: " + a.TaskID), nil
		})
}

func (s *Server) toolGetTaskGraph(langs ...locale.Lang) actool.CoreTool {
	return roTool("get_task_graph", locale.Text(locale.First(langs), "Read a task's graph overview by task_id: asset counts, frontier, findings, coverage, and related situation data, like graph_overview."),
		objSchema(map[string]any{"task_id": strParam(locale.Text(locale.First(langs), "Task ID"))}, "task_id"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			return s.delegateToTask(ctx, in, (*agent.ToolSet).GraphOverviewTool)
		})
}

func (s *Server) toolListTaskFindings(langs ...locale.Lang) actool.CoreTool {
	return roTool("list_task_findings", locale.Text(locale.First(langs), "Read confirmed findings for task_id, including flags/PoC and id/task_id/intent_id/vulnclass/severity/summary/status."),
		objSchema(map[string]any{"task_id": strParam(locale.Text(locale.First(langs), "Task ID"))}, "task_id"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			return s.delegateToTask(ctx, in, (*agent.ToolSet).ListFindingsTool)
		})
}

func (s *Server) toolAddHint(langs ...locale.Lang) actool.CoreTool {
	return wrTool("add_task_hint", locale.Text(locale.First(langs), "Add strategic hints to a task for its planner's next round.\n")+
		locale.Text(locale.First(langs), "Prefer one hints batch; returned ids match length/order and failed entries are 0. For one hint, omit hints and provide top-level text."),
		objSchema(map[string]any{
			"task_id":      strParam(locale.Text(locale.First(langs), "Task ID")),
			"hints":        map[string]any{"type": "array", "description": locale.Text(locale.First(langs), "Preferred hints array; each item uses text/asset_ids/traffic_refs."), "items": objSchema(map[string]any{"text": strParam(locale.Text(locale.First(langs), "Hint text")), "asset_ids": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}}, "traffic_refs": agent.HintTrafficSchema(locale.First(langs))})},
			"text":         strParam(locale.Text(locale.First(langs), "Single hint text")),
			"traffic_refs": agent.HintTrafficSchema(locale.First(langs)),
			"asset_ids":    map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": locale.Text(locale.First(langs), "Optional anchored asset IDs in that task (zero/one/many)")},
		}, "task_id"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			return s.delegateToTask(ctx, in, (*agent.ToolSet).AddHintTool)
		})
}

func (s *Server) toolGetWorkerTrace(langs ...locale.Lang) actool.CoreTool {
	return roTool("get_task_worker_trace",
		locale.Text(locale.First(langs), "Inspect a worker intent's trace in a task. get_task_worker_trace(task_id,intent_id) returns step summaries; add step_ids for full content, at most the first five per call."),
		objSchema(map[string]any{
			"task_id":   strParam(locale.Text(locale.First(langs), "Task ID")),
			"intent_id": map[string]any{"type": "integer", "description": locale.Text(locale.First(langs), "Intent ID for work in that task")},
			"step_ids":  map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": locale.Text(locale.First(langs), "Optional step IDs for full content, at most five; excess IDs appear in omitted_step_ids")},
		}, "task_id", "intent_id"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			return s.delegateToTask(ctx, in, (*agent.ToolSet).GetWorkerTraceTool)
		})
}

func (s *Server) toolListWorkerTraces(langs ...locale.Lang) actool.CoreTool {
	return roTool("list_task_worker_traces", locale.Text(locale.First(langs), "List executed intents and step counts for a task to identify traces worth inspecting with get_task_worker_trace."),
		objSchema(map[string]any{"task_id": strParam(locale.Text(locale.First(langs), "Task ID"))}, "task_id"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			return s.delegateToTask(ctx, in, (*agent.ToolSet).ListWorkerTracesTool)
		})
}

func (s *Server) toolSearchWorkerTraces(langs ...locale.Lang) actool.CoreTool {
	return roTool("search_task_worker_traces", locale.Text(locale.First(langs), "Search all worker traces in a task by keyword, returning matching step summaries and intent_id."),
		objSchema(map[string]any{"task_id": strParam(locale.Text(locale.First(langs), "Task ID")), "q": strParam(locale.Text(locale.First(langs), "Search keyword"))}, "task_id", "q"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			return s.delegateToTask(ctx, in, (*agent.ToolSet).SearchWorkerTracesTool)
		})
}

func (s *Server) toolGetTaskNodeDetail(langs ...locale.Lang) actool.CoreTool {
	return roTool("get_task_node_detail",
		locale.Text(locale.First(langs), "Read full exploration-node content for a task (finding/fact/intent/goal: summary, detail, evidence, PoC). id is the exploration node ID from report_finding or list_task_findings, not an asset ID. Read full finding evidence before writing its report."),
		objSchema(map[string]any{
			"task_id": strParam(locale.Text(locale.First(langs), "Task ID")),
			"id":      map[string]any{"type": "integer", "description": locale.Text(locale.First(langs), "Exploration node ID, not an asset ID")},
		}, "task_id", "id"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			return s.delegateToTask(ctx, in, (*agent.ToolSet).NodeDetailTool)
		})
}

// toolUpdateFindingReport writes/overwrites a finding's detailed Markdown report.
// finding_id is the id report_finding returned ("finding recorded: <id>", the
// finding node id). The write (SetFindingReportByNodeID) is keyed by node_id and
// task-agnostic, so this host tool needs no task_id / exploration store.
func (s *Server) toolUpdateFindingReport(langs ...locale.Lang) actool.CoreTool {
	return wrTool("update_finding_report",
		locale.Text(locale.First(langs), "Write/update a registered finding's detailed Markdown report, replacing the previous report. finding_id takes the exploration node ID in report_finding's \"finding recorded: <id>\" line. Include overview, impact, reproduction, evidence/PoC, and remediation."),
		objSchema(map[string]any{
			"finding_id":       map[string]any{"type": "integer", "description": locale.Text(locale.First(langs), "Target exploration finding node ID returned by report_finding")},
			"report":           strParam(locale.Text(locale.First(langs), "Complete detailed report in Markdown")),
			"evidence_version": map[string]any{"type": "integer", "description": locale.Text(locale.First(langs), "Evidence version returned by get_finding_traffic, preventing reports from overwriting newer evidence changes")},
		}, "finding_id", "report"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				EvidenceVersion *int64          `json:"evidence_version"`
				FindingID       json.RawMessage `json:"finding_id"`
				Report          string          `json:"report"`
			}
			_ = json.Unmarshal(in, &a)
			nodeID := parseProfileID(a.FindingID) // 复用「数字或数字字符串」解析
			if nodeID <= 0 {
				return actool.Errorf(locale.Text(locale.First(langs), "Invalid finding_id")), nil
			}
			n, err := s.m.pg.SetFindingReportVersionByNodeID(ctx, nodeID, a.Report, a.EvidenceVersion)
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			if n == 0 {
				return actool.Errorf(fmt.Sprintf(locale.Text(locale.First(langs), "No finding record for finding_id=%d; register it first with report_finding"), nodeID)), nil
			}
			return actool.Text(fmt.Sprintf("finding %d report updated (%d chars)", nodeID, len(a.Report))), nil
		})
}

// deriveTaskStatus mirrors listTasks' status derivation for the list_tasks tool.
func (s *Server) deriveTaskStatus(t *Task) string {
	lifecycle := t.lifecycleSnapshot()
	switch {
	case isTerminalStatus(lifecycle.Status):
		return lifecycle.Status
	case lifecycle.Paused || s.engine.IsPaused(t.ID):
		return "paused"
	case s.engine.ReadyFor(t) && s.engine.Started(t.ID):
		return "running"
	}
	return "created"
}

// orchestrationToolSeeds seeds the cross-task tools into the tools table so they
// are bindable per-agent (default: bound to nobody — opt-in for orchestration
// agents). First-insert only, like the traffic seeds.
func (s *Server) seedOrchestrationTools() {
	// task-op + platform tools default-bind to the built-in Auto agent (它天生用来
	// 操作平台)。SeedTool 首插入生效;老库已 seed 的行由 seedAutoDefaultBindings 补绑。
	autoAgents, _ := json.Marshal([]string{"auto"})
	for _, t := range s.orchestrationTools(locale.En) {
		schema, _ := json.Marshal(t.InputSchema())
		bindings := autoAgents
		if t.Name() == "bind_finding_traffic" {
			bindings = json.RawMessage(`["reporter"]`)
		}
		_ = s.m.PG().SeedTool(t.Name(), t.Description(), schema, bindings)
	}
	for _, t := range s.platformTools(locale.En) {
		schema, _ := json.Marshal(t.InputSchema())
		_ = s.m.PG().SeedTool(t.Name(), t.Description(), schema, autoAgents)
	}
	s.refreshBuiltinToolSchemas()
	s.seedAutoDefaultBindings()
	s.seedPlannerDefaultBindings()
	s.seedPlannerListAssetsBinding()
	s.seedCompanyScopeRebind()
	s.seedWorkerReadToolsUnbind() // list_facts/list_companies/list_worker_traces 从 worker 默认解绑(一次性)
	s.seedWorkerReadbackRebind()  // 修复旧迁移误删：把 search_all_worker_traces/get_worker_trace/node_detail 补绑回 worker(一次性)
	s.seedAutoReportFindingBinding()
	s.unbindGoalMetDefault()
	s.reseedGoalsPrompt()             // goals 提示词加入「抽操作约束」步 → 旧库追加一版新默认(一次性)
	s.reseedMainAgentPrompt()         // mainagent 提示词加入「目标达成后 add_intent 反问是否建目标」(一次性)
	s.reseedPlannerPrompt()           // planner 提示词:重写「0 意图」正当理由 + 加量化验收核对(一次性)
	s.reseedWorkerPrompt()            // worker 提示词:加否定结论证据门槛(一次性)
	s.seedReporterAgent()             // 预置「报告撰写」agent + 工具绑定 + finding 触发器(一次性)
	s.upgradeReporterTriggerMessage() // 老库补迁移:让 reporter 回传 evidence_version(一次性)
	s.seedFindingTrafficTools()       // 增加可选证据参数及只读证据工具，保留用户配置
	s.seedFindingWorkflowTools()
	// 注：pentest 的默认工具绑定无需迁移——BuiltinToolSeeds 在全新初始化时就把
	// list_assets/insert_assets/report_finding/list_findings/list_companies 连同
	// pentest 一起 seed 好了（项目尚无旧库，不做迁移）。
}

// refreshBuiltinToolSchemas propagates code schema/description changes on the
// orchestration + platform tools into already-seeded rows ONCE per version flag —
// SeedTool is first-insert-only, so a new param (e.g. spawn_task 的 llm_profile) never
// reaches an old DB otherwise. Preserves each tool's agent binding + enabled flag.
// Bump the flag whenever these tools' schemas/descriptions change in code.
func (s *Server) refreshBuiltinToolSchemas() {
	const flag = "tool_schema_refresh_v7_list_facts_paging"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	tools := append(s.orchestrationTools(locale.En), s.platformTools(locale.En)...)
	for _, t := range tools {
		schema, _ := json.Marshal(t.InputSchema())
		if err := s.m.pg.RefreshToolDefaults(t.Name(), t.Description(), schema); err != nil {
			log.Printf("[tools] refresh %s schema failed: %v", t.Name(), err)
		}
	}
	// 同时把部分内置 agent 工具刷成代码默认：
	//   - goal_met：旧库 seed 的描述带“结束本轮规划”的误导，会让 planner 把它当成
	//     “结束空轮”的手段、刚开跑就误判整个任务完成。
	//   - insert_assets：新增 related 入参(标记资产是否与当前任务相关、决定是否入覆盖度)，
	//     SeedTool 首插入only，旧库已 seed 的 schema 否则收不到这个新参数。
	//   - list_facts：改为分页，新增 limit/before/q 入参；旧库已 seed 的空 schema 否则
	//     在工具管理页显示「无参数」，模型也拿不到这几个参数说明。
	refreshBuiltin := map[string]bool{"goal_met": true, "insert_assets": true, "list_facts": true}
	for _, sd := range agent.BuiltinToolSeeds() {
		if !refreshBuiltin[sd.Key] {
			continue
		}
		schema, _ := json.Marshal(sd.Schema)
		if err := s.m.pg.RefreshToolDefaults(sd.Key, sd.Desc, schema); err != nil {
			log.Printf("[tools] refresh %s desc failed: %v", sd.Key, err)
		}
	}
	_ = s.m.pg.SetSetting(flag, "true")
	log.Print(locale.Text(locale.ServerDefault(), "[tools] Refreshed orchestration/platform schemas to code defaults (one-time)"))
}

// unbindGoalMetDefault removes goal_met's default "planner" binding ONCE (guarded by
// a settings flag), so existing DBs match the new default of NO agent. goal_met bypasses
// per-goal prove_goal to declare the whole task done — powerful/risky and redundant with
// the prove_goal→auto-complete path — so it ships unbound; users can re-bind it per agent
// in the UI. A user's own binding to another agent is untouched (we only strip planner).
func (s *Server) unbindGoalMetDefault() {
	const flag = "goal_met_unbind_default_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	if err := s.m.pg.RemoveAgentFromTool("planner", "goal_met"); err != nil {
		log.Printf(locale.Text(locale.ServerDefault(), "[tools] Could not unbind goal_met from planner: %v"), err)
		return
	}
	_ = s.m.pg.SetSetting(flag, "true")
}

// reseedGoalsPrompt 把 goals 目标拆解器的提示词刷成【当前代码默认】——因为默认正文新增了
// 「先抽操作约束(set_constraints)再拆目标」这一步,而 SeedPromptIfEmpty 首插入only,旧库
// 已有的 version 1 收不到这步。这里用版本管理【追加一个新版本】并切过去(ResetPromptToDefault),
// 旧的版本仍保留在历史里,用户若自定义过可从版本记录找回。settings flag 守卫 → 只做一次;
// 以后默认再变就 bump 这个 flag。全新库无需处理(SeedPromptIfEmpty 已 seed 最新默认)。
func (s *Server) reseedGoalsPrompt() {
	const flag = "goals_prompt_constraint_step_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	defer func() { _ = s.m.pg.SetSetting(flag, "true") }() // 无论成功与否只尝试一次
	a, err := s.m.pg.GetAgentByKey("goals")
	if err != nil || a == nil {
		return // 全新库尚未建 agent 行时,seedPrompts 会直接 seed 最新默认,无需此迁移
	}
	tmpl := agent.BuiltinPromptSeeds()["goals"]
	if tmpl == "" {
		return
	}
	// 全新库 seedPrompts 已 seed 最新默认 → 当前版本已等于代码默认,不必再追加重复版本。
	if cur, err := s.m.pg.CurrentPrompt(a.ID); err == nil && cur == tmpl {
		return
	}
	if _, err := s.m.pg.ResetPromptToDefault(a.ID, tmpl); err != nil {
		log.Printf(locale.Text(locale.ServerDefault(), "[prompts] Could not refresh the goals default: %v"), err)
		return
	}
	log.Print(locale.Text(locale.ServerDefault(), "[prompts] Added goals default version with constraint extraction (one-time)"))
}

// reseedMainAgentPrompt 把 mainagent 提示词刷成【当前代码默认】——默认正文新增了「目标全部
// 达成后 add_intent 直投意图时,反问人是否登记为正式目标」这段引导,而 SeedPromptIfEmpty 首插入
// only,旧库已有版本收不到。用版本管理【追加一个新版本】并切过去(ResetPromptToDefault),旧版本仍
// 保留在历史里,用户若自定义过可从版本记录找回。settings flag 守卫 → 只做一次。全新库无需处理
// (SeedPromptIfEmpty 已 seed 最新默认)。与 reseedGoalsPrompt 完全同构。
func (s *Server) reseedMainAgentPrompt() {
	const flag = "mainagent_prompt_goalless_intent_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	defer func() { _ = s.m.pg.SetSetting(flag, "true") }() // 无论成功与否只尝试一次
	a, err := s.m.pg.GetAgentByKey("mainagent")
	if err != nil || a == nil {
		return // 全新库尚未建 agent 行时,seedPrompts 会直接 seed 最新默认,无需此迁移
	}
	tmpl := agent.BuiltinPromptSeeds()["mainagent"]
	if tmpl == "" {
		return
	}
	// 全新库 seedPrompts 已 seed 最新默认 → 当前版本已等于代码默认,不必再追加重复版本。
	if cur, err := s.m.pg.CurrentPrompt(a.ID); err == nil && cur == tmpl {
		return
	}
	if _, err := s.m.pg.ResetPromptToDefault(a.ID, tmpl); err != nil {
		log.Printf(locale.Text(locale.ServerDefault(), "[prompts] Could not refresh the mainagent default: %v"), err)
		return
	}
	log.Print(locale.Text(locale.ServerDefault(), "[prompts] Added mainagent default version with confirmation before new post-completion goals (one-time)"))
}

// reseedPlannerPrompt 把 planner 提示词刷成【当前代码默认】——默认正文做了精简重构,并把「克制」降级为
// 仅去重、新增「深度优先于覆盖度」「硬底线:目标未达成且无在跑意图必须产出」、给否定结论复核加上界。
// 每次默认有实质变更就 bump 下面的 flag(当前 v2)让存量旧库再刷一次。SeedPromptIfEmpty 首插入only,旧库已有版本收不到,故用版本管理
// 【追加一个新版本】并切过去(ResetPromptToDefault),旧版本仍保留在历史里,用户若自定义过可从版本记录
// 找回。settings flag 守卫 → 只做一次。全新库无需处理(SeedPromptIfEmpty 已 seed 最新默认)。与
// reseedGoalsPrompt 完全同构。
func (s *Server) reseedPlannerPrompt() {
	const flag = "planner_prompt_compact_realistic_v2"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	defer func() { _ = s.m.pg.SetSetting(flag, "true") }() // 无论成功与否只尝试一次
	a, err := s.m.pg.GetAgentByKey("planner")
	if err != nil || a == nil {
		return // 全新库尚未建 agent 行时,seedPrompts 会直接 seed 最新默认,无需此迁移
	}
	tmpl := agent.BuiltinPromptSeeds()["planner"]
	if tmpl == "" {
		return
	}
	// 全新库 seedPrompts 已 seed 最新默认 → 当前版本已等于代码默认,不必再追加重复版本。
	if cur, err := s.m.pg.CurrentPrompt(a.ID); err == nil && cur == tmpl {
		return
	}
	if _, err := s.m.pg.ResetPromptToDefault(a.ID, tmpl); err != nil {
		log.Printf(locale.Text(locale.ServerDefault(), "[prompts] Could not refresh the planner default: %v"), err)
		return
	}
	log.Print(locale.Text(locale.ServerDefault(), "[prompts] Added planner default with deduplication, depth priority, and bounded negative rechecks (one-time)"))
}

// reseedWorkerPrompt 把 worker 提示词刷成【当前代码默认】——默认正文 record_fact 段删掉了「否定类结论
// 写观察+试探性读法」整句、并把 confidence(observed/inferred)与「是否穷尽本意图手段」解耦(这些易误导规划者),
// 同时把 facts 数组分条收紧为「彼此完全独立、无法归并」的极少数例外。bump flag 至 v3 让存量旧库再刷一次。
// SeedPromptIfEmpty 首插入only,旧库已有版本收不到,故用版本管理【追加一个新版本】并切过去,旧版本仍保留在历史里可找回。
// settings flag 守卫 → 只做一次。全新库无需处理。与 reseedGoalsPrompt 完全同构。
func (s *Server) reseedWorkerPrompt() {
	const flag = "worker_prompt_compact_v4"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	defer func() { _ = s.m.pg.SetSetting(flag, "true") }() // 无论成功与否只尝试一次
	a, err := s.m.pg.GetAgentByKey("worker")
	if err != nil || a == nil {
		return // 全新库尚未建 agent 行时,seedPrompts 会直接 seed 最新默认,无需此迁移
	}
	tmpl := agent.BuiltinPromptSeeds()["worker"]
	if tmpl == "" {
		return
	}
	// 全新库 seedPrompts 已 seed 最新默认 → 当前版本已等于代码默认,不必再追加重复版本。
	if cur, err := s.m.pg.CurrentPrompt(a.ID); err == nil && cur == tmpl {
		return
	}
	if _, err := s.m.pg.ResetPromptToDefault(a.ID, tmpl); err != nil {
		log.Printf(locale.Text(locale.ServerDefault(), "[prompts] Could not refresh the worker default: %v"), err)
		return
	}
	log.Print(locale.Text(locale.ServerDefault(), "[prompts] Added worker default with focused list_assets/list_findings context guidance (one-time)"))
}

// reporterToolCallMessage 必须无条件要求先读一次 get_finding_traffic 再写报告。
// 该工具是只读的、「不依赖捕获开关」,自动绑定关不关都能读到人工绑定的证据。若这里
// 写成「启用自动绑定才读」,默认关闭配置下 reporter 就不会传 evidence_version,
// SetFindingReportVersionByNodeID 便按 legacy 语义写 -1,漏洞详情与 Markdown 导出
// 从此常驻「证据已变更，报告待更新」,而 UI 上没有任何入口能把它清掉。
const reporterToolCallMessage = "A finding was just registered by report_finding. Read finding_id (independent finding record) and finding_node_id (exploration node) from the returned JSON. " +
	"Use get_finding_traffic(finding_id) to read the evidence list and version; an empty list is valid and does not block reporting. " +
	"If run guidance enables automatic binding, verify and bind this finding's traffic before reading. Use finding_node_id for node details. " +
	"Finally save with update_finding_report(finding_id=finding_node_id, report, evidence_version=the version actually read). " +
	"evidence_version is required or the report remains stale. Never mix the two ID namespaces."

// 旧版触发消息(0.3.8 及更早)。只有仍与它逐字相同的记录才会被迁移覆盖，用户改过的保持原样。
const reporterToolCallMessageV1 = "A finding was just registered by report_finding. Extract its finding_id " +
	"(the number in \"finding recorded: <id>\") and task ID from the trigger context, write its detailed report, " +
	"then save with update_finding_report(finding_id, report)."

// upgradeReporterTriggerMessage 把老库里仍是默认文案的 reporter 触发消息刷成新版本。
// seedReporterAgent 受 reporter_agent_seed_v1 守卫且只在新建 agent 时写触发器，所以
// 升级上来的库拿不到新文案 —— 工具 schema 由 seedFindingTrafficTools 补齐了
// evidence_version，但没有任何东西告诉 reporter 去用它。一次性，且只覆盖未被改动的文案。
func (s *Server) upgradeReporterTriggerMessage() {
	const flag = "reporter_trigger_evidence_version_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	defer func() { _ = s.m.pg.SetSetting(flag, "true") }() // 只尝试一次
	triggers, err := s.m.pg.ListTriggersFor("reporter")
	if err != nil {
		log.Printf(locale.Text(locale.ServerDefault(), "[reporter] Could not read triggers: %v"), err)
		return
	}
	for _, t := range triggers {
		if !t.OnToolCall || !isLegacyReporterTrigger(t.ToolCallMessage) {
			continue // 用户改过或不是 finding 触发器，不动。
		}
		t.ToolCallMessage = reporterToolCallMessage
		if err := s.m.pg.UpdateTrigger(t); err != nil {
			log.Printf(locale.Text(locale.ServerDefault(), "[reporter] Could not upgrade trigger message: %v"), err)
			return
		}
		log.Print(locale.Text(locale.ServerDefault(), "[reporter] Trigger message now reads and returns evidence_version"))
	}
}

// seedReporterAgent 预置一个「报告撰写」自定义 agent(builtin=false，可在 UI 编辑/删除)：
// 绑定 update_finding_report + 任务查询工具，并挂一个「report_finding 被调用即触发」的
// 触发器 —— 每登记一个漏洞就唤起它写详细报告。一次性(settings flag 守卫)：用户删掉后不再重建。
// 依赖：orchestration 工具已在本函数上方 SeedTool 入库，故绑定得上。
func (s *Server) seedReporterAgent() {
	const flag = "reporter_agent_seed_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	defer func() { _ = s.m.pg.SetSetting(flag, "true") }() // 无论成功与否只尝试一次

	if exist, _ := s.m.pg.GetAgentByKey("reporter"); exist != nil {
		return // key 已被占用(用户手建过)——不覆盖
	}
	a, err := s.m.pg.CreateAgent("reporter", "Report writer",
		"Writes detailed vulnerability reports: triggered by findings, reads evidence and execution traces, then saves a Markdown report.")
	if err != nil {
		log.Printf(locale.Text(locale.ServerDefault(), "[reporter] Could not create agent: %v"), err)
		return
	}
	if err := s.m.pg.SeedPromptIfEmpty(a.ID, agent.ReporterDefaultPrompt); err != nil {
		log.Printf(locale.Text(locale.ServerDefault(), "[reporter] Could not seed prompt: %v"), err)
	}
	// 触发运行策略：parallel + none —— 一漏洞一报告、多个 finding 并发各写各的。
	// merge 必须为 none：否则(默认 all)一波 finding 会被合并成一次运行，并行就没意义。
	// maxParallel=5：同时最多 5 个报告会话，避免瞬时太多 LLM 调用。
	if err := s.m.pg.SetAgentTriggerBehavior("reporter", "parallel", "none", 5); err != nil {
		log.Printf(locale.Text(locale.ServerDefault(), "[reporter] Could not set trigger behavior: %v"), err)
	}
	// 绑定它需要的工具：写报告 + 读证据/执行过程/态势。
	if err := s.m.pg.AddAgentToToolBinding("reporter", []string{
		"update_finding_report", "get_task_node_detail", "list_task_findings",
		"get_task_worker_trace", "list_task_worker_traces", "search_task_worker_traces",
		"get_task_graph",
	}); err != nil {
		log.Printf(locale.Text(locale.ServerDefault(), "[reporter] Could not bind tools: %v"), err)
	}
	// 触发器：report_finding 被调用即触发（工具返回 "finding recorded: <id>" 带上 finding_id，
	// 任务 id 也在触发消息里）。
	if _, err := s.m.pg.CreateTrigger(&db.AgentTrigger{
		AgentKey:        "reporter",
		Enabled:         true,
		OnToolCall:      true,
		ToolNames:       []string{"report_finding"},
		ToolCallMessage: reporterToolCallMessage,
	}); err != nil {
		log.Printf(locale.Text(locale.ServerDefault(), "[reporter] Could not create trigger: %v"), err)
	}
	log.Print(locale.Text(locale.ServerDefault(), "[reporter] Seeded report-writer agent and finding trigger"))
}

// seedAutoReportFindingBinding adds "auto" to report_finding's binding ONCE so
// conversation-context agents can call it without requiring an intent_id.
func (s *Server) seedAutoReportFindingBinding() {
	const flag = "auto_report_finding_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	if err := s.m.pg.AddAgentToToolBinding("auto", []string{"report_finding"}); err != nil {
		log.Printf(locale.Text(locale.ServerDefault(), "[auto] Could not bind report_finding by default: %v"), err)
		return
	}
	_ = s.m.pg.SetSetting(flag, "true")
}

// seedPlannerDefaultBindings adds "planner" to report_finding's binding ONCE
// (guarded by a settings flag), so existing DBs — whose report_finding row was
// seeded as worker-only — also let the planner record findings. Fresh DBs already
// get it via PlannerTools(); this only backfills without overriding a user unbind.
func (s *Server) seedPlannerDefaultBindings() {
	const flag = "planner_report_finding_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	if err := s.m.pg.AddAgentToToolBinding("planner", []string{"report_finding"}); err != nil {
		log.Printf(locale.Text(locale.ServerDefault(), "[planner] Could not bind report_finding by default: %v"), err)
		return
	}
	_ = s.m.pg.SetSetting(flag, "true")
}

// seedPlannerListAssetsBinding adds "planner" to list_assets's binding ONCE
// (guarded by a settings flag), so existing DBs — whose list_assets row was seeded
// as auto/pentest-only — also let the planner query the asset store by DSL. Fresh
// DBs already get it via PlannerTools(); this only backfills without overriding a
// user unbind.
func (s *Server) seedPlannerListAssetsBinding() {
	const flag = "planner_list_assets_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	if err := s.m.pg.AddAgentToToolBinding("planner", []string{"list_assets"}); err != nil {
		log.Printf(locale.Text(locale.ServerDefault(), "[planner] Could not bind list_assets by default: %v"), err)
		return
	}
	_ = s.m.pg.SetSetting(flag, "true")
}

// seedCompanyScopeRebind changes add_company_scope's default binding ONCE on
// existing DBs (guarded by a settings flag): the tool moves off worker and onto
// planner — defining a company's asset scope is a planning/main/auto concern, not
// something a worker does mid-exploration. Fresh DBs already get planner via
// PlannerTools() and lack worker via WorkerTools(); this only backfills old rows.
// One-shot + flag-guarded so a user who later re-binds worker isn't overridden.
func (s *Server) seedCompanyScopeRebind() {
	const flag = "company_scope_rebind_v1" // worker→planner 默认绑定切换
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	if err := s.m.pg.AddAgentToToolBinding("planner", []string{"add_company_scope"}); err != nil {
		log.Printf(locale.Text(locale.ServerDefault(), "[planner] Could not bind add_company_scope by default: %v"), err)
		return
	}
	if err := s.m.pg.RemoveAgentFromTool("worker", "add_company_scope"); err != nil {
		log.Printf(locale.Text(locale.ServerDefault(), "[worker] Could not unbind add_company_scope: %v"), err)
		return
	}
	_ = s.m.pg.SetSetting(flag, "true")
}

// seedWorkerReadToolsUnbind strips the read-context tools off worker's default
// binding ONCE on existing DBs (guarded by a settings flag): a worker executes one
// intent and writes back — reading facts/companies and listing all workers' traces is
// a planning/main concern, not the executor's. Fresh DBs already lack these via
// WorkerTools(); this only backfills old rows without overriding a user who
// deliberately re-binds worker. Each RemoveAgentFromTool is per-tool +
// membership-guarded, so planner/mainagent bindings of the same tool are untouched.
//
// NOTE: search_all_worker_traces / get_worker_trace / node_detail are intentionally NOT
// unbound — worker owns them for cross-work look-back + node drill-down (see WorkerTools).
// They used to be in this list back when worker lacked them; seedWorkerReadbackRebind
// repairs DBs whose old run stripped them.
func (s *Server) seedWorkerReadToolsUnbind() {
	const flag = "worker_readtools_unbind_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	for _, k := range []string{
		"list_facts", "list_companies", "list_worker_traces",
	} {
		if err := s.m.pg.RemoveAgentFromTool("worker", k); err != nil {
			log.Printf(locale.Text(locale.ServerDefault(), "[worker] Could not unbind %s: %v"), k, err)
			return // 出错则不落 flag，下次启动重试
		}
	}
	_ = s.m.pg.SetSetting(flag, "true")
}

// seedWorkerReadbackRebind re-binds the cross-work look-back / drill-down tools onto
// worker ONCE (guarded by a settings flag): an earlier seedWorkerReadToolsUnbind wrongly
// stripped search_all_worker_traces / get_worker_trace / node_detail from worker after
// they had been added to WorkerTools(), so any DB that ran that migration lost them.
// Fresh DBs already have them via WorkerTools() and this is a harmless no-op there.
// One-shot + flag-guarded so a user who later deliberately unbinds them isn't overridden.
func (s *Server) seedWorkerReadbackRebind() {
	const flag = "worker_readback_rebind_v2" // v2: 追加 node_detail
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	if err := s.m.pg.AddAgentToToolBinding("worker", []string{
		"search_all_worker_traces", "get_worker_trace", "node_detail",
	}); err != nil {
		log.Printf(locale.Text(locale.ServerDefault(), "[worker] Could not bind trace/detail tools: %v"), err)
		return // 出错则不落 flag，下次启动重试
	}
	_ = s.m.pg.SetSetting(flag, "true")
}

// seedAutoDefaultBindings adds "auto" to the task-op + platform tools' bindings
// ONCE (guarded by a settings flag), so existing DBs whose tool rows were seeded
// before Auto existed still give Auto its default toolset — without re-adding it
// after a user deliberately unbinds.
func (s *Server) seedAutoDefaultBindings() {
	const flag = "auto_default_bindings_v3" // v3: 替换旧资产工具名，加入 insert_assets/add_company_scope
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	keys := make([]string, 0, len(platformToolKeys)+12)
	for _, t := range s.orchestrationTools(locale.En) {
		keys = append(keys, t.Name())
	}
	keys = append(keys, platformToolKeys...)
	// 资产工具：Auto 操作平台常要看/登记资产、管理公司范围。
	keys = append(keys, "insert_assets", "add_company_scope", "list_assets")
	if err := s.m.pg.AddAgentToToolBinding("auto", keys); err != nil {
		log.Printf(locale.Text(locale.ServerDefault(), "[auto] Default tool binding failed: %v"), err)
		return
	}
	_ = s.m.pg.SetSetting(flag, "true")
}

// Match the exact historical stock trigger without rewriting custom trigger text.
func isLegacyReporterTrigger(text string) bool {
	if text == reporterToolCallMessageV1 {
		return true
	}
	digest := sha256.Sum256([]byte(text))
	return hex.EncodeToString(digest[:]) == "c10b22c2daf51a0e4998fe0e84a2e22baf70927a6f7368874e1e1e0b0eacb02b"
}
