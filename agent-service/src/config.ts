import "dotenv/config";

function required(name: string, fallback?: string): string {
	const value = process.env[name] ?? fallback;
	if (value === undefined || value === "") {
		throw new Error(`缺少环境变量 ${name}（参考 .env.example）`);
	}
	return value;
}

export const config = {
	port: Number(process.env.AGENT_SERVICE_PORT ?? 8082),
	db: {
		host: required("DB_HOST", "127.0.0.1"),
		port: Number(process.env.DB_PORT ?? 3307),
		user: required("DB_USER", "root"),
		password: process.env.DB_PASS ?? "rootpassword",
		database: required("DB_NAME", "erp_db"),
	},
	jwtSecret: required("JWT_SECRET", "erp_wonchon_default_secret_key_2026_please_change"),
	aiConfigSecret: process.env.AI_CONFIG_SECRET || process.env.JWT_SECRET || "",
	erpApiUrl: (process.env.ERP_API_URL ?? "http://127.0.0.1:8081").replace(/\/$/, ""),
	envModel: {
		apiKey: process.env.AI_API_KEY ?? "",
		model: process.env.AI_MODEL ?? "",
		url: process.env.AI_API_URL ?? "",
	},
};

export type AppConfig = typeof config;
