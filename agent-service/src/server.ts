import Fastify from "fastify";
import multipart from "@fastify/multipart";
import crypto from "node:crypto";
import type { RowDataPacket, ResultSetHeader } from "mysql2/promise";
import { type AgentEvent } from "@earendil-works/pi-agent-core";
import type { ImageContent } from "@earendil-works/pi-ai";
import { config } from "./config.js";
import { ensureSchema, pool } from "./db.js";
import { requireAdmin, type AgentIdentity } from "./auth.js";
import { createErpAgent, historyToAgentMessages } from "./agent-factory.js";
import { extractPdfText } from "./pdf.js";

const app = Fastify({ logger: false, bodyLimit: 48 * 1024 * 1024 });
await app.register(multipart, { limits: { fileSize: 40 * 1024 * 1024, files: 20 } });

await ensureSchema();

const VALID_THINKING = new Set(["", "auto", "quick", "standard", "deep"]);

interface MessageRow {
	id: number;
	session_id: number;
	role: string;
	content: string;
	sources: string | null;
	evidence: string | null;
	status: string | null;
	action_id: number | null;
	run_id: string | null;
	attachments?: unknown[];
	created_at?: string;
}

function rowToMessage(row: RowDataPacket, attachments: unknown[] = []): MessageRow {
	return {
		id: Number(row.id),
		session_id: Number(row.session_id),
		role: String(row.role),
		content: String(row.content ?? ""),
		sources: row.sources == null ? null : String(row.sources),
		evidence: row.evidence == null ? null : String(row.evidence),
		status: row.status == null ? null : String(row.status),
		action_id: row.action_id == null ? null : Number(row.action_id),
		run_id: row.run_id == null ? null : String(row.run_id),
		attachments,
		created_at: row.created_at ? String(row.created_at) : undefined,
	};
}

async function ownedSession(id: number, owner: string): Promise<RowDataPacket | undefined> {
	const [rows] = await pool.query<RowDataPacket[]>(
		"SELECT id, owner, debug, created_at FROM ai_agent_sessions WHERE id = ? AND owner = ? LIMIT 1",
		[id, owner],
	);
	return rows[0];
}

// ---------------------------------------------------------------------------
// 会话
// ---------------------------------------------------------------------------

app.post("/api/ai/agent/sessions", async (request, reply) => {
	const identity = requireAdmin(request, reply);
	if (!identity) return;
	const [result] = await pool.execute<ResultSetHeader>(
		"INSERT INTO ai_agent_sessions (owner, debug, created_at, updated_at) VALUES (?, 0, NOW(3), NOW(3))",
		[identity.username],
	);
	reply.send({ code: 200, data: { id: result.insertId } });
});

app.get("/api/ai/agent/sessions/:id", async (request, reply) => {
	const identity = requireAdmin(request, reply);
	if (!identity) return;
	const id = Number((request.params as { id: string }).id);
	const session = await ownedSession(id, identity.username);
	if (!session) {
		reply.code(404).send({ code: 404, msg: "会话不存在" });
		return;
	}
	const [messageRows] = await pool.query<RowDataPacket[]>(
		"SELECT * FROM (SELECT id, session_id, role, content, sources, evidence, status, action_id, run_id, created_at FROM ai_agent_messages WHERE session_id = ? ORDER BY id DESC LIMIT 100) t ORDER BY id ASC",
		[id],
	);
	const messageIds = messageRows.map((row) => Number(row.id));
	let attachmentsByMessage = new Map<number, unknown[]>();
	if (messageIds.length > 0) {
		const [attachmentRows] = await pool.query<RowDataPacket[]>(
			"SELECT id, message_id, name, relative_path, media_type, size, truncated FROM ai_agent_attachments WHERE message_id IN (?)",
			[messageIds],
		);
		for (const row of attachmentRows) {
			const messageId = Number(row.message_id);
			const list = attachmentsByMessage.get(messageId) ?? [];
			list.push({ id: Number(row.id), name: row.name, relative_path: row.relative_path, media_type: row.media_type, size: Number(row.size), truncated: !!row.truncated });
			attachmentsByMessage.set(messageId, list);
		}
	}
	const [actionRows] = await pool.query<RowDataPacket[]>(
		"SELECT * FROM (SELECT id, kind, payload, status, target_id, expires_at, created_at FROM ai_agent_actions WHERE session_id = ? ORDER BY id DESC LIMIT 30) t ORDER BY id ASC",
		[id],
	);
	const assistantIds = messageRows.filter((row) => row.role === "assistant").map((row) => Number(row.id));
	const feedbacks: Record<string, unknown> = {};
	if (assistantIds.length > 0) {
		const [feedbackRows] = await pool.query<RowDataPacket[]>(
			"SELECT assistant_message_id, rating, category, comment FROM ai_agent_feedbacks WHERE owner = ? AND assistant_message_id IN (?)",
			[identity.username, assistantIds],
		);
		for (const row of feedbackRows) {
			feedbacks[String(row.assistant_message_id)] = { rating: row.rating, category: row.category, comment: row.comment };
		}
	}
	reply.send({
		code: 200,
		data: {
			id,
			messages: messageRows.map((row) => rowToMessage(row, attachmentsByMessage.get(Number(row.id)) ?? [])),
			actions: actionRows,
			feedbacks,
		},
	});
});

// ---------------------------------------------------------------------------
// 待办摘要
// ---------------------------------------------------------------------------

app.get("/api/ai/agent/brief", async (request, reply) => {
	const identity = requireAdmin(request, reply);
	if (!identity) return;
	const [unpaidRows] = await pool.query<RowDataPacket[]>(
		"SELECT COUNT(*) AS c FROM orders WHERE order_status = 1 AND payment_status = 1 AND actual_pay_amount > deposit_amount",
	);
	const [subsidyRows] = await pool.query<RowDataPacket[]>(
		"SELECT COUNT(*) AS c FROM orders WHERE order_status = 1 AND subsidy_status IN (1, 2)",
	);
	reply.send({
		code: 200,
		data: [
			{ key: "unpaid", label: "待收尾款订单", count: Number(unpaidRows[0].c), path: "/admin/orders" },
			{ key: "subsidy", label: "待处理国补订单", count: Number(subsidyRows[0].c), path: "/admin/subsidy" },
		],
	});
});

// ---------------------------------------------------------------------------
// 长期记忆
// ---------------------------------------------------------------------------

app.get("/api/ai/agent/memories", async (request, reply) => {
	const identity = requireAdmin(request, reply);
	if (!identity) return;
	const [rows] = await pool.query<RowDataPacket[]>(
		"SELECT id, kind, content, source, evidence, created_at, updated_at FROM ai_agent_memories WHERE owner = ? ORDER BY updated_at DESC LIMIT 200",
		[identity.username],
	);
	reply.send({ code: 200, data: rows });
});

app.post("/api/ai/agent/memories", async (request, reply) => {
	const identity = requireAdmin(request, reply);
	if (!identity) return;
	const body = (request.body ?? {}) as { content?: string; kind?: string };
	const content = (body.content ?? "").trim().slice(0, 500);
	if (!content) {
		reply.code(400).send({ code: 400, msg: "记忆内容不能为空" });
		return;
	}
	const [result] = await pool.execute<ResultSetHeader>(
		"INSERT INTO ai_agent_memories (owner, kind, content, source, created_at, updated_at) VALUES (?, ?, ?, 'manual', NOW(3), NOW(3))",
		[identity.username, (body.kind ?? "preference").slice(0, 20), content],
	);
	reply.send({ code: 200, data: { id: result.insertId } });
});

app.put("/api/ai/agent/memories/:id", async (request, reply) => {
	const identity = requireAdmin(request, reply);
	if (!identity) return;
	const id = Number((request.params as { id: string }).id);
	const body = (request.body ?? {}) as { content?: string };
	const content = (body.content ?? "").trim().slice(0, 500);
	if (!content) {
		reply.code(400).send({ code: 400, msg: "记忆内容不能为空" });
		return;
	}
	await pool.execute("UPDATE ai_agent_memories SET content = ?, updated_at = NOW(3) WHERE id = ? AND owner = ?", [
		content,
		id,
		identity.username,
	]);
	reply.send({ code: 200, msg: "已更新" });
});

app.delete("/api/ai/agent/memories/:id", async (request, reply) => {
	const identity = requireAdmin(request, reply);
	if (!identity) return;
	const id = Number((request.params as { id: string }).id);
	await pool.execute("DELETE FROM ai_agent_memories WHERE id = ? AND owner = ?", [id, identity.username]);
	reply.send({ code: 200, msg: "已删除" });
});

// ---------------------------------------------------------------------------
// 消息反馈
// ---------------------------------------------------------------------------

app.post("/api/ai/agent/messages/:id/feedback", async (request, reply) => {
	const identity = requireAdmin(request, reply);
	if (!identity) return;
	const messageId = Number((request.params as { id: string }).id);
	const body = (request.body ?? {}) as { rating?: string; category?: string; comment?: string };
	const rating = (body.rating ?? "").slice(0, 12);
	if (!["up", "down"].includes(rating)) {
		reply.code(400).send({ code: 400, msg: "反馈无效" });
		return;
	}
	await pool.execute(
		"INSERT INTO ai_agent_feedbacks (assistant_message_id, owner, rating, category, comment, created_at, updated_at) VALUES (?, ?, ?, ?, ?, NOW(3), NOW(3)) ON DUPLICATE KEY UPDATE rating = VALUES(rating), category = VALUES(category), comment = VALUES(comment), updated_at = NOW(3)",
		[messageId, identity.username, rating, (body.category ?? "").slice(0, 32), (body.comment ?? "").slice(0, 500)],
	);
	reply.send({ code: 200, msg: "已记录反馈" });
});

// ---------------------------------------------------------------------------
// 草稿确认（支出）：校验草稿 → 调 Go 记账接口 → 消费草稿
// ---------------------------------------------------------------------------

app.post("/api/ai/agent/actions/:id/confirm", async (request, reply) => {
	const identity = requireAdmin(request, reply);
	if (!identity) return;
	const actionId = Number((request.params as { id: string }).id);
	const payloadEdits = (request.body ?? {}) as Record<string, unknown>;

	const [rows] = await pool.query<RowDataPacket[]>(
		"SELECT id, kind, payload, status, expires_at FROM ai_agent_actions WHERE id = ? AND owner = ? LIMIT 1",
		[actionId, identity.username],
	);
	const action = rows[0];
	if (!action) {
		reply.code(404).send({ code: 404, msg: "Agent 草稿不存在" });
		return;
	}
	if (action.kind !== "expense") {
		reply.code(400).send({ code: 400, msg: "订单草稿请通过订单接口确认" });
		return;
	}
	if (action.status !== "pending" || new Date(String(action.expires_at)).getTime() < Date.now()) {
		reply.code(409).send({ code: 409, msg: "Agent 草稿已使用或已过期" });
		return;
	}
	const [claimed] = await pool.execute<ResultSetHeader>(
		"UPDATE ai_agent_actions SET status = 'processing', updated_at = NOW(3) WHERE id = ? AND status = 'pending'",
		[actionId],
	);
	if (claimed.affectedRows !== 1) {
		reply.code(409).send({ code: 409, msg: "Agent 草稿已使用" });
		return;
	}

	let draft: Record<string, unknown>;
	try {
		draft = JSON.parse(String(action.payload));
	} catch {
		await pool.execute("UPDATE ai_agent_actions SET status = 'pending' WHERE id = ?", [actionId]);
		reply.code(500).send({ code: 500, msg: "草稿数据无效" });
		return;
	}
	const body: Record<string, unknown> = { ...draft, ...payloadEdits, entry_type: 2 };
	delete body.agent_action_id;

	try {
		const response = await fetch(`${config.erpApiUrl}/api/bookkeeping/entries`, {
			method: "POST",
			headers: { "Content-Type": "application/json", Authorization: `Bearer ${identity.bearer}` },
			body: JSON.stringify(body),
		});
		const result = (await response.json()) as { code?: number; msg?: string; data?: { id?: number } };
		if (!response.ok || (result.code !== 200 && result.code !== undefined)) {
			throw new Error(result.msg || `记账接口返回 HTTP ${response.status}`);
		}
		const targetId = result.data?.id ?? 0;
		await pool.execute("UPDATE ai_agent_actions SET status = 'saved', target_id = ?, updated_at = NOW(3) WHERE id = ?", [
			targetId,
			actionId,
		]);
		reply.send({ code: 200, data: { id: targetId, path: "/admin/bookkeeping" } });
	} catch (cause) {
		await pool.execute("UPDATE ai_agent_actions SET status = 'pending', updated_at = NOW(3) WHERE id = ?", [actionId]);
		reply.code(409).send({ code: 409, msg: cause instanceof Error ? cause.message : "保存失败，请重试" });
	}
});

// ---------------------------------------------------------------------------
// 聊天（SSE 流式，pi agent loop）
// ---------------------------------------------------------------------------

interface AttachmentInput {
	name: string;
	mediaType: string;
	size: number;
	buffer: Buffer;
	/** 抽取文本超上限被截断（当前仅 PDF 路径会产生），落库到 ai_agent_attachments.truncated */
	truncated: boolean;
}

function sseWrite(raw: import("node:http").ServerResponse, event: string, data: unknown): void {
	raw.write(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`);
}

app.post("/api/ai/agent/sessions/:id/messages", async (request, reply) => {
	const identity = requireAdmin(request, reply);
	if (!identity) return;
	const sessionId = Number((request.params as { id: string }).id);
	const session = await ownedSession(sessionId, identity.username);
	if (!session) {
		reply.code(404).send({ code: 404, msg: "会话不存在" });
		return;
	}

	let message = "";
	let thinking = "auto";
	const attachments: AttachmentInput[] = [];

	const contentType = request.headers["content-type"] ?? "";
	if (contentType.includes("multipart/form-data")) {
		for await (const part of request.parts()) {
			if (part.type === "file") {
				const buffer = await part.toBuffer();
				attachments.push({ name: part.filename, mediaType: part.mimetype, size: buffer.length, buffer, truncated: false });
			} else {
				const value = String((part.fields as Record<string, unknown>)[part.fieldname] ?? "");
				if (part.fieldname === "message") message = value;
				if (part.fieldname === "thinking") thinking = value;
			}
		}
	} else {
		const body = (request.body ?? {}) as Record<string, unknown>;
		message = String(body.message ?? "");
		thinking = String(body.thinking ?? "auto");
	}
	if (!VALID_THINKING.has(thinking)) {
		reply.code(400).send({ code: 400, msg: "思考等级无效" });
		return;
	}
	message = message.trim();
	if (!message && attachments.length === 0) {
		reply.code(400).send({ code: 400, msg: "请输入不超过500字的问题" });
		return;
	}
	if (message.length > 500) {
		reply.code(400).send({ code: 400, msg: "请输入不超过500字的问题" });
		return;
	}

	// 附件解析：图片直接作为视觉输入；纯文本与 PDF 抽文本后拼进消息；其余不支持。
	const imageBlocks: ImageContent[] = [];
	const textChunks: string[] = [];
	for (const attachment of attachments) {
		if (attachment.mediaType.startsWith("image/")) {
			imageBlocks.push({
				type: "image",
				data: attachment.buffer.toString("base64"),
				mimeType: attachment.mediaType,
			});
		} else if (attachment.mediaType.startsWith("text/") || /\.(txt|md|csv|json)$/i.test(attachment.name)) {
			textChunks.push(`\n\n[附件 ${attachment.name}]\n${attachment.buffer.toString("utf8").slice(0, 20000)}`);
		} else if (attachment.mediaType === "application/pdf" || /\.pdf$/i.test(attachment.name)) {
			// PDF：抽取纯文本走 textChunks 路径，超上限截断并在落库时置 truncated 标记
			try {
				const { text, truncated } = await extractPdfText(attachment.buffer);
				if (!text) {
					reply.code(400).send({ code: 400, msg: `PDF 未能解析出文本：${attachment.name}（可能是扫描件，请转图片后上传）` });
					return;
				}
				attachment.truncated = truncated;
				const suffix = truncated ? "（内容过长已截断）" : "";
				textChunks.push(`\n\n[附件 ${attachment.name}${suffix}]\n${text}`);
			} catch {
				reply.code(400).send({ code: 400, msg: `PDF 解析失败：${attachment.name}，请确认文件未损坏后重试` });
				return;
			}
		} else {
			reply.code(400).send({ code: 400, msg: `暂不支持附件格式：${attachment.name}（支持图片、纯文本与 PDF）` });
			return;
		}
	}

	// 最近的对话作为 pi 转录种子。
	const [recentRows] = await pool.query<RowDataPacket[]>(
		"SELECT id, role, content FROM (SELECT id, role, content FROM ai_agent_messages WHERE session_id = ? ORDER BY id DESC LIMIT 16) t ORDER BY id ASC",
		[sessionId],
	);

	const runId = crypto.randomUUID();
	const [userInsert] = await pool.execute<ResultSetHeader>(
		"INSERT INTO ai_agent_messages (session_id, role, content, run_id, created_at) VALUES (?, 'user', ?, ?, NOW(3))",
		[sessionId, message, runId],
	);
	const userMessageId = userInsert.insertId;
	for (const attachment of attachments) {
		await pool.execute(
			"INSERT INTO ai_agent_attachments (session_id, message_id, owner, name, relative_path, media_type, size, data, truncated, created_at, updated_at) VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, NOW(3), NOW(3))",
			[sessionId, userMessageId, identity.username, attachment.name.slice(0, 255), attachment.mediaType.slice(0, 120), attachment.size, attachment.buffer, attachment.truncated ? 1 : 0],
		);
	}

	// SSE 响应头。
	const raw = reply.raw;
	reply.hijack();
	raw.writeHead(200, {
		"Content-Type": "text/event-stream; charset=utf-8",
		"Cache-Control": "no-cache",
		Connection: "keep-alive",
		"X-Accel-Buffering": "no",
	});
	sseWrite(raw, "status", { stage: "started", user_message_id: userMessageId });

	const abortController = new AbortController();
	request.raw.on("close", () => {
		if (!raw.writableEnded) abortController.abort();
	});

	const fullText = message + textChunks.join("");
	const contentBlocks: ({ type: "text"; text: string } | ImageContent)[] = [];
	if (fullText) contentBlocks.push({ type: "text", text: fullText });
	contentBlocks.push(...imageBlocks);

	let assistantText = "";
	let stopStatus = "complete";
	let errorMessage = "";
	const evidence: { id: string; source: string; result: string }[] = [];
	const sources: string[] = [];
	const runStartedAt = Date.now();
	let createdActionId = 0;

	try {
		const history = historyToAgentMessages(recentRows);
		const { agent } = await createErpAgent(identity.username, identity.bearer, sessionId, thinking, history);

		agent.subscribe((event: AgentEvent) => {
			if (event.type === "message_update") {
				for (const block of event.message.content) {
					if (typeof block === "object" && block.type === "text") {
						assistantText = block.text;
					}
				}
				const last = event.message.content[event.message.content.length - 1];
				if (typeof last === "object" && last.type === "text" && event.assistantMessageEvent?.type === "text_delta") {
					sseWrite(raw, "delta", { text: assistantText });
				}
				return;
			}
			if (event.type === "tool_execution_start") {
				sseWrite(raw, "tool", { name: event.toolName, state: "start" });
				return;
			}
			if (event.type === "tool_execution_end") {
				const text = (event.result?.content ?? [])
					.map((block: { type: string; text?: string }) => (block.type === "text" ? block.text ?? "" : ""))
					.join("\n");
				evidence.push({ id: event.toolCallId, source: event.toolName, result: text.slice(0, 4000) });
				if (!sources.includes(event.toolName)) sources.push(event.toolName);
				sseWrite(raw, "tool", { name: event.toolName, state: "end", isError: event.isError });
				if (event.result?.structuredContent && typeof (event.result.structuredContent as { action_id?: number }).action_id === "number") {
					createdActionId = (event.result.structuredContent as { action_id: number }).action_id;
				}
			}
		});

		const timeout = setTimeout(() => abortController.abort(), 4 * 60 * 1000);
		try {
			await agent.prompt([{ role: "user", content: contentBlocks, timestamp: Date.now() }]);
		} finally {
			clearTimeout(timeout);
		}
	} catch (cause) {
		stopStatus = "failed";
		errorMessage = cause instanceof Error ? cause.message : String(cause);
	}

	if (!assistantText && errorMessage) {
		assistantText = "Agent 暂时无法处理，请稍后重试。";
	}

	const sourcesJson = JSON.stringify(sources);
	const evidenceJson = JSON.stringify(evidence);
	const [assistantInsert] = await pool.execute<ResultSetHeader>(
		"INSERT INTO ai_agent_messages (session_id, role, content, sources, evidence, status, action_id, run_id, created_at) VALUES (?, 'assistant', ?, ?, ?, ?, ?, ?, NOW(3))",
		[sessionId, assistantText, sourcesJson, evidenceJson, stopStatus === "complete" ? "complete" : "incomplete", createdActionId || null, runId],
	);
	const assistantMessageId = assistantInsert.insertId;

	// 完整 pi 转录存档（追溯用）。
	const transcript = { run_id: runId, thinking, duration_ms: Date.now() - runStartedAt, error: errorMessage || undefined };
	await pool
		.execute("INSERT INTO agent_transcripts (assistant_message_id, owner, payload, created_at) VALUES (?, ?, ?, NOW(3))", [
			assistantMessageId,
			identity.username,
			JSON.stringify(transcript),
		])
		.catch(() => undefined);

	sseWrite(raw, "done", {
		message: {
			id: assistantMessageId,
			session_id: sessionId,
			role: "assistant",
			content: assistantText,
			sources: sourcesJson,
			evidence: evidenceJson,
			status: stopStatus === "complete" ? "complete" : "incomplete",
			action_id: createdActionId || null,
			run_id: runId,
		},
		user_message: { id: userMessageId, session_id: sessionId, role: "user", content: message, run_id: runId },
		action: createdActionId ? { id: createdActionId, kind: "draft", status: "pending" } : null,
	});
	raw.end();
});

// ---------------------------------------------------------------------------
// 启动
// ---------------------------------------------------------------------------

await app.listen({ port: config.port, host: "0.0.0.0" });
console.log(`[agent-service] pi 核心 Agent 服务已启动：http://127.0.0.1:${config.port}`);
