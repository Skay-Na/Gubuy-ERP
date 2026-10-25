import { Type } from "@earendil-works/pi-ai";
import type { AgentTool } from "@earendil-works/pi-agent-core";
import type { JsonValue } from "@earendil-works/pi-ai";
import type { ResultSetHeader } from "mysql2/promise";
import { pool } from "./db.js";
import { config } from "./config.js";

export interface ToolContext {
	/** 当前管理员的 Bearer Token，透传给 Go 后端做鉴权。 */
	bearer: string;
	sessionId: number;
	owner: string;
}

const DATE_PATTERN = /^\d{4}-\d{2}-\d{2}$/;

function today(): string {
	return new Date().toISOString().slice(0, 10);
}

interface GoAgentToolResponse {
	summary?: string;
	items?: unknown;
	[key: string]: unknown;
}

async function callGoAgentTool(path: string, bearer: string, query: Record<string, string | number | undefined>): Promise<GoAgentToolResponse> {
	const params = new URLSearchParams();
	for (const [key, value] of Object.entries(query)) {
		if (value !== undefined && value !== "") params.set(key, String(value));
	}
	const response = await fetch(`${config.erpApiUrl}/api/ai/agent-tools/${path}?${params.toString()}`, {
		headers: { Authorization: `Bearer ${bearer}` },
	});
	const body = (await response.json()) as { code?: number; msg?: string; data?: GoAgentToolResponse };
	if (!response.ok || body.code !== 200) {
		throw new Error(body.msg || `工具查询失败（HTTP ${response.status}）`);
	}
	return body.data ?? {};
}

function textResult(text: string, data?: unknown) {
	const result: {
		content: [{ type: "text"; text: string }];
		details: Record<string, never>;
		structuredContent?: JsonValue;
		isError?: boolean;
	} = { content: [{ type: "text", text }], details: {} };
	return data === undefined ? result : { ...result, structuredContent: data as JsonValue };
}

async function insertAction(ctx: ToolContext, kind: string, payload: string): Promise<number> {
	const [result] = await pool.execute<ResultSetHeader>(
		"INSERT INTO ai_agent_actions (session_id, owner, kind, payload, status, expires_at, created_at, updated_at) VALUES (?, ?, ?, ?, 'pending', DATE_ADD(NOW(3), INTERVAL 24 HOUR), NOW(3), NOW(3))",
		[ctx.sessionId, ctx.owner, kind, payload],
	);
	return result.insertId;
}

/** 构建本次会话可用的 ERP 业务工具集（pi AgentTool）。 */
export function createTools(ctx: ToolContext): AgentTool[] {
	const readTool = (
		name: string,
		label: string,
		description: string,
		properties: Record<string, any>,
	): AgentTool => ({
		name,
		label,
		description,
		parameters: Type.Object(properties),
		async execute(_toolCallId, params) {
			const query = params as Record<string, string | number | undefined>;
			const path = name.replace(/^query_/, "").replace(/_/g, "-");
			const data = await callGoAgentTool(path, ctx.bearer, query);
			const summary = typeof data.summary === "string" && data.summary ? data.summary : "查询完成";
			return textResult(summary, data.items ?? data);
		},
	});

	return [
		readTool(
			"query_product_inventory",
			"查询商品库存",
			"按商品名称关键词查询当前门店与云仓可售库存（不含样机）。keyword 必填。",
			{ keyword: Type.String({ description: "商品名称关键词" }), limit: Type.Optional(Type.Number({ description: "最多返回条数，默认 20" })) },
		),
		readTool(
			"query_sales_summary",
			"查询销量汇总",
			"统计指定时间范围内有效订单的商品销量和订单数。日期格式 YYYY-MM-DD，结束日期含当天。",
			{
				start_date: Type.String({ description: "开始日期 YYYY-MM-DD" }),
				end_date: Type.String({ description: "结束日期 YYYY-MM-DD（含当天）" }),
				limit: Type.Optional(Type.Number()),
			},
		),
		readTool(
			"query_sales_trend",
			"查询销量趋势",
			"将所选时间段与前一个等长时间段比较有效订单销量，返回销量变化最大的商品；end_date 为排他边界。分析\"最近\"变化时建议取最近 14 天并与前 14 天比较。",
			{
				start_date: Type.String({ description: "开始日期 YYYY-MM-DD" }),
				end_date: Type.String({ description: "结束日期 YYYY-MM-DD（排他）" }),
				limit: Type.Optional(Type.Number()),
			},
		),
		readTool(
			"query_inventory_sales",
			"查询库存与销量组合",
			"将当前可售库存与指定时间范围内的销量及订单数合并（最多 100 个商品），适合\"库存够不够卖\"类问题。",
			{
				start_date: Type.String({ description: "开始日期 YYYY-MM-DD" }),
				end_date: Type.String({ description: "结束日期 YYYY-MM-DD（含当天）" }),
				limit: Type.Optional(Type.Number()),
			},
		),
		readTool(
			"query_unpaid_orders",
			"查询未结清订单",
			"查询未结清的有效订单（待收尾款）。customer 可选，填客户姓名关键词。",
			{
				customer: Type.Optional(Type.String({ description: "客户姓名关键词" })),
				limit: Type.Optional(Type.Number({ description: "最多返回条数，默认 20" })),
			},
		),
		{
			name: "create_order_draft",
			label: "创建订单草稿",
			description:
				"根据用户提供的信息创建一张订单草稿，仅生成待确认卡片，不会直接保存订单。缺少客户姓名、商品、数量或单价时不要调用，先追问。",
			parameters: Type.Object({
				customer_name: Type.String({ description: "客户姓名" }),
				customer_phone: Type.Optional(Type.String({ description: "客户电话" })),
				items: Type.Array(
					Type.Object({
						name: Type.String({ description: "商品名称" }),
						quantity: Type.Number({ description: "数量" }),
						unit_price: Type.Number({ description: "单价（元）" }),
					}),
					{ description: "商品明细，最多 12 项" },
				),
			}),
			async execute(_toolCallId, rawParams) {
				const params = rawParams as { customer_name?: string; customer_phone?: string; items: { name: string; quantity: number; unit_price: number }[] };
				const items = params.items.slice(0, 12).map((item) => ({
					name: String(item.name).slice(0, 120),
					quantity: Number(item.quantity) || 1,
					unit_price: Number(item.unit_price) || 0,
				}));
				if (!params.customer_name?.trim() || items.length === 0) {
					return { ...textResult("缺少客户姓名或商品明细，未创建草稿"), isError: true };
				}
				const payload = JSON.stringify({
					customer_name: params.customer_name.trim(),
					customer_phone: params.customer_phone?.trim() ?? "",
					items,
					payment_method: 0,
					delivery_method: 0,
					account_id: 0,
					payment_mode: "full",
					deposit_amount: 0,
				});
				const actionId = await insertAction(ctx, "order", payload);
				return textResult(
					`订单草稿 #${actionId} 已生成（${items.length} 项商品，客户 ${params.customer_name.trim()}）。请提醒用户在卡片中核对信息并点击确认后才会真正保存订单。`,
					{ action_id: actionId },
				);
			},
		},
		{
			name: "create_expense_draft",
			label: "创建支出草稿",
			description:
				"创建一笔经营支出草稿，仅生成待确认卡片，不会直接入账。渠道奖励支出不允许通过这里登记。缺少金额、类别或账户时先追问。账户 ID：1=支付宝 2=微信 3=公户 4=现金 7=经营开销。",
			parameters: Type.Object({
				amount: Type.Number({ description: "金额（元）" }),
				category: Type.String({ description: "支出类别，如 房租/进货/工资" }),
				account_id: Type.Number({ description: "资金账户 ID" }),
				counterparty: Type.Optional(Type.String({ description: "收款方/往来方" })),
				remark: Type.Optional(Type.String({ description: "备注" })),
				occurred_at: Type.Optional(Type.String({ description: "发生日期 YYYY-MM-DD，默认今天" })),
			}),
			async execute(_toolCallId, rawParams) {
				const params = rawParams as { amount: number; category: string; account_id: number; counterparty?: string; remark?: string; occurred_at?: string };
				const amount = Number(params.amount);
				if (!(amount > 0) || !params.category?.trim() || !params.account_id) {
					return { ...textResult("缺少金额、类别或资金账户，未创建草稿"), isError: true };
				}
				const occurredAt = DATE_PATTERN.test(params.occurred_at ?? "") ? params.occurred_at : today();
				const payload = JSON.stringify({
					entry_type: 2,
					amount,
					category: params.category.trim().slice(0, 50),
					counterparty: (params.counterparty ?? "").slice(0, 100),
					remark: (params.remark ?? "").slice(0, 255),
					occurred_at: occurredAt,
					account_id: Number(params.account_id),
				});
				const actionId = await insertAction(ctx, "expense", payload);
				return textResult(
					`支出草稿 #${actionId} 已生成（¥${amount.toFixed(2)}，${params.category.trim()}）。请提醒用户在卡片中核对后点击确认才会计入账目。`,
					{ action_id: actionId },
				);
			},
		},
		{
			name: "save_long_term_memory",
			label: "保存长期记忆",
			description: "当用户明确表达偏好、背景或长期约定时，把这条信息保存到长期记忆，之后每次对话都会参考。",
			parameters: Type.Object({
				content: Type.String({ description: "要记住的内容，一句话，不超过 200 字" }),
			}),
			async execute(_toolCallId, rawParams) {
				const params = rawParams as { content: string };
				const content = params.content.trim().slice(0, 200);
				if (!content) {
					return { ...textResult("内容为空，未保存"), isError: true };
				}
				await pool.execute(
					"INSERT INTO ai_agent_memories (owner, kind, content, source, created_at, updated_at) VALUES (?, 'preference', ?, 'agent', NOW(3), NOW(3))",
					[ctx.owner, content],
				);
				return textResult(`已保存到长期记忆：${content}`);
			},
		},
	];
}
