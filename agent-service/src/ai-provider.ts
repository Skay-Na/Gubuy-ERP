import crypto from "node:crypto";
import { stream as openaiCompletionsStream } from "@earendil-works/pi-ai/api/openai-completions";
import type { StreamFn } from "@earendil-works/pi-agent-core";
import type { Model } from "@earendil-works/pi-ai";
import { pool } from "./db.js";
import { config } from "./config.js";
import type { RowDataPacket } from "mysql2/promise";

/** 与 Go 后端 ai_settings_controller.go 保持一致的服务商目录（baseUrl 为 API 根地址，SDK 自动追加 /chat/completions）。 */
const PROVIDER_BASE_URLS: Record<string, { cn: string; global?: string }> = {
	deepseek: { cn: "https://api.deepseek.com" },
	minimax: { cn: "https://api.minimaxi.com/v1", global: "https://api.minimax.io/v1" },
	mimo: { cn: "https://api.xiaomimimo.com/v1" },
	qwen: {
		cn: "https://dashscope.aliyuncs.com/compatible-mode/v1",
		global: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
	},
};

const PROVIDER_DEFAULT_MODELS: Record<string, string> = {
	deepseek: "deepseek-chat",
	minimax: "MiniMax-M3",
	mimo: "mimo-v2.6-flash",
	qwen: "qwen3-vl-plus",
};

export interface AgentModelRuntime {
	model: Model<"openai-completions">;
	apiKey: string;
	provider: string;
}

/**
 * 解密 Go 后端保存的 AI 服务商密钥。
 * 算法与 ai_settings_controller.go 完全一致：
 * key = SHA-256("erpwonchon-ai-provider-key-v1:" + secret)，AES-256-GCM，nonce 前置，base64。
 */
function decryptAIKey(encoded: string): string {
	const secret = config.aiConfigSecret;
	if (secret.length < 24) {
		throw new Error("AI 配置加密密钥未配置或过短（需要 AI_CONFIG_SECRET 或 24 位以上 JWT_SECRET）");
	}
	const key = crypto.createHash("sha256").update(`erpwonchon-ai-provider-key-v1:${secret}`).digest();
	const data = Buffer.from(encoded, "base64");
	const nonceSize = 12;
	if (data.length <= nonceSize) {
		throw new Error("AI 密钥数据无效");
	}
	const decipher = crypto.createDecipheriv("aes-256-gcm", key, data.subarray(0, nonceSize));
	decipher.setAuthTag(data.subarray(data.length - 16));
	let plain: Buffer;
	try {
		plain = Buffer.concat([decipher.update(data.subarray(nonceSize, data.length - 16)), decipher.final()]);
	} catch {
		throw new Error(
			"无法解密 AI 密钥：当前服务端加密密钥与保存时不一致。请在系统设置重新输入服务商 API Key 并保存（保存后新旧 Agent 服务均可正常使用）",
		);
	}
	return plain.toString("utf8");
}

function normalizeBaseUrl(url: string): string {
	return url.replace(/\/chat\/completions\/?$/, "").replace(/\/$/, "");
}

function buildModel(provider: string, model: string, baseUrl: string): Model<"openai-completions"> {
	return {
		id: model,
		name: model,
		api: "openai-completions",
		provider,
		baseUrl,
		reasoning: false,
		input: ["text", "image"],
		cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
		contextWindow: 131072,
		maxTokens: 8192,
	};
}

function envRuntime(): AgentModelRuntime | null {
	const { apiKey, model, url } = config.envModel;
	if (!apiKey || !model) return null;
	const baseUrl = normalizeBaseUrl(url || "https://api.openai.com/v1");
	return { model: buildModel("env", model, baseUrl), apiKey, provider: "env" };
}

/** 读取系统设置中启用的 AI 服务商；未启用任何服务商时回退到环境变量配置。 */
export async function resolveModelRuntime(): Promise<AgentModelRuntime> {
	// 注意：GORM 对 AIProviderConfig 的默认表名是 a_iprovider_configs
	const [rows] = await pool.query<RowDataPacket[]>(
		"SELECT provider, model, region, encrypted_key FROM a_iprovider_configs WHERE active = 1 LIMIT 1",
	);
	const saved = rows[0];
	if (saved) {
		const urls = PROVIDER_BASE_URLS[String(saved.provider)];
		if (!urls) throw new Error("当前 AI 服务商无效");
		const region = String(saved.region || "cn");
		const baseUrl = region === "global" && urls.global ? urls.global : urls.cn;
		const apiKey = decryptAIKey(String(saved.encrypted_key));
		const model = String(saved.model || PROVIDER_DEFAULT_MODELS[String(saved.provider)] || "");
		return { model: buildModel(String(saved.provider), model, baseUrl), apiKey, provider: String(saved.provider) };
	}
	const runtime = envRuntime();
	if (!runtime) {
		throw new Error("请在系统设置中配置并启用 AI 服务商");
	}
	return runtime;
}

/** pi agent 的 StreamFn：统一走 OpenAI 兼容 chat/completions 适配器。 */
export function createStreamFn(getRuntime: () => AgentModelRuntime): StreamFn {
	return (model, context, options) =>
		openaiCompletionsStream(model as Model<"openai-completions">, context, {
			...options,
			apiKey: getRuntime().apiKey,
		});
}
