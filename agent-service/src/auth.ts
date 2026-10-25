import jwt from "jsonwebtoken";
import type { FastifyReply, FastifyRequest } from "fastify";
import { config } from "./config.js";

export interface AgentIdentity {
	username: string;
	/** 透传给 Go 后端的 Bearer Token。 */
	bearer: string;
}

const EMP_PREFIX = "emp_";

export function requireAdmin(request: FastifyRequest, reply: FastifyReply): AgentIdentity | undefined {
	const header = request.headers.authorization ?? "";
	const [scheme, token] = header.split(" ");
	if (scheme !== "Bearer" || !token) {
		reply.code(401).send({ code: 401, msg: "未登录，无权访问" });
		return undefined;
	}
	try {
		const claims = jwt.verify(token, config.jwtSecret) as { username?: string };
		if (!claims.username || claims.username.startsWith(EMP_PREFIX)) {
			reply.code(403).send({ code: 403, msg: "需要管理员权限" });
			return undefined;
		}
		return { username: claims.username, bearer: token };
	} catch {
		reply.code(401).send({ code: 401, msg: "无效的 Token 或 Token 已过期" });
		return undefined;
	}
}
