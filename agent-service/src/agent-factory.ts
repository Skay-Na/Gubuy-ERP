import { Agent } from "@earendil-works/pi-agent-core";
import type { AgentMessage, AgentTool } from "@earendil-works/pi-agent-core";
import type { Model } from "@earendil-works/pi-ai";
import type { RowDataPacket } from "mysql2/promise";
import { pool } from "./db.js";
import { createStreamFn, resolveModelRuntime, type AgentModelRuntime } from "./ai-provider.js";
import { buildSystemPrompt } from "./prompts.js";
import { createTools } from "./tools.js";

async function loadMemories(owner: string): Promise<string[]> {
	const [rows] = await pool.query<RowDataPacket[]>(
		"SELECT content FROM ai_agent_memories WHERE owner = ? ORDER BY updated_at DESC LIMIT 30",
		[owner],
	);
	return rows.map((row) => String(row.content));
}

export interface ErpAgentHandle {
	agent: Agent;
	runtime: AgentModelRuntime;
	tools: AgentTool[];
}

/**
 * 用 pi agent 核心创建一次会话的执行器：
 * - 模型来自系统设置中启用的 OpenAI 兼容服务商（pi-ai openai-completions 适配器）
 * - 工具为 ERP 业务工具（查询走 Go 后端，草稿/记忆写共享 MySQL）
 * - 长期记忆注入系统提示词
 */
export async function createErpAgent(
	owner: string,
	bearer: string,
	sessionId: number,
	thinking: string,
	history: AgentMessage[],
): Promise<ErpAgentHandle> {
	const runtime = await resolveModelRuntime();
	const memories = await loadMemories(owner);
	const tools = createTools({ bearer, sessionId, owner });
	const thinkingHint =
		thinking === "deep"
			? "\n用户要求深度分析：请充分调用工具交叉验证后再回答。"
			: thinking === "standard"
				? "\n用户要求标准分析：按需调用工具后给出结构化结论。"
				: "";
	const agent = new Agent({
		initialState: {
			model: runtime.model as Model<string>,
			systemPrompt: buildSystemPrompt(new Date(), memories) + thinkingHint,
			tools,
			messages: history,
		},
		streamFn: createStreamFn(() => runtime),
		toolExecution: "sequential",
	});
	return { agent, runtime, tools };
}

/** 把 ai_agent_messages 里的最近对话映射成 pi 消息转录。 */
export function historyToAgentMessages(rows: RowDataPacket[]): AgentMessage[] {
	const messages: AgentMessage[] = [];
	for (const row of rows) {
		const role = String(row.role);
		const content = String(row.content ?? "");
		if (!content) continue;
		if (role === "user") {
			messages.push({ role: "user", content: [{ type: "text", text: content }], timestamp: Date.now() });
		} else if (role === "assistant") {
			messages.push({
				role: "assistant",
				content: [{ type: "text", text: content }],
				api: "openai-completions",
				provider: "history",
				model: "history",
				usage: {
					input: 0,
					output: 0,
					cacheRead: 0,
					cacheWrite: 0,
					totalTokens: 0,
					cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 },
				},
				stopReason: "stop",
				timestamp: Date.now(),
			});
		}
	}
	return messages;
}
