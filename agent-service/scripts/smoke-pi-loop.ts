/**
 * pi 核心链路冒烟测试（不调真实 LLM）：
 * 用 pi-ai 的 faux provider 编排两步响应（工具调用 → 文本回答），
 * 验证 Agent loop → 工具 schema 校验 → Go 后端 agent-tools 接口 → 最终回答 全链路。
 *
 * 前置：Go 后端已启动（默认 127.0.0.1:8081），JWT_SECRET 与后端一致。
 * 运行：npx tsx scripts/smoke-pi-loop.ts
 */
import "dotenv/config";
import { Agent } from "@earendil-works/pi-agent-core";
import { fauxAssistantMessage, fauxProvider, fauxToolCall, fauxText, type Model } from "@earendil-works/pi-ai";
import jwt from "jsonwebtoken";
import { createTools } from "../src/tools.js";
import { config } from "../src/config.js";

async function main() {
	const bearer = jwt.sign({ username: "admin", exp: Math.floor(Date.now() / 1000) + 600 }, config.jwtSecret);
	const tools = createTools({ bearer, sessionId: 0, owner: "smoke-test" });
	const toolByName = new Map(tools.map((tool) => [tool.name, tool]));

	const faux = fauxProvider({ api: "openai-completions", provider: "smoke" });
	const model = faux.models[0] as Model<string>;
	const streamFn = (m: Model<string>, context: any, options: any) => faux.provider.streamSimple(m, context, options);

	// 第一步：模型请求调用工具；第二步：基于工具结果给出回答
	faux.setResponses([
		fauxAssistantMessageWithToolCall("query_unpaid_orders", { limit: 5 }),
		fauxAssistantMessageWithText("冒烟完成：已通过工具查询到未结清订单。"),
	]);

	const agent = new Agent({
		initialState: { model, systemPrompt: "冒烟测试", tools },
		streamFn,
		toolExecution: "sequential",
	});

	let toolRan = false;
	agent.subscribe((event) => {
		if (event.type === "tool_execution_end") {
			toolRan = true;
			const text = (event.result?.content ?? [])
				.map((block: any) => (block.type === "text" ? block.text : ""))
				.join("");
			console.log(`[tool] ${event.toolName} isError=${event.isError} → ${text.slice(0, 120)}`);
		}
		if (event.type === "message_end" && event.message.role === "assistant") {
			for (const block of event.message.content) {
				if (block.type === "text" && block.text) console.log(`[assistant] ${block.text.slice(0, 160)}`);
			}
		}
	});

	await agent.prompt("查询未结清订单");
	if (!toolRan) throw new Error("冒烟失败：工具未被执行");
	console.log("\nSMOKE OK：pi loop → 工具 → Go 后端 全链路通过");
	process.exit(0);
}

function fauxAssistantMessageWithToolCall(name: string, args: Record<string, unknown>) {
	const blocks = [fauxText(`调用 ${name}`), fauxToolCall(name, args, { id: `call_${name}` })];
	return fauxAssistantMessage(blocks);
}

function fauxAssistantMessageWithText(text: string) {
	return fauxAssistantMessage([fauxText(text)]);
}

main().catch((error) => {
	console.error("SMOKE FAILED:", error);
	process.exit(1);
});
