import { createPool, type Pool, type PoolConnection } from "mysql2/promise";
import { config } from "./config.js";

export const pool: Pool = createPool({
	host: config.db.host,
	port: config.db.port,
	user: config.db.user,
	password: config.db.password,
	database: config.db.database,
	waitForConnections: true,
	connectionLimit: 10,
	dateStrings: false,
	charset: "utf8mb4",
});

export async function withTx<T>(work: (conn: PoolConnection) => Promise<T>): Promise<T> {
	const conn = await pool.getConnection();
	try {
		await conn.beginTransaction();
		const result = await work(conn);
		await conn.commit();
		return result;
	} catch (error) {
		await conn.rollback();
		throw error;
	} finally {
		conn.release();
	}
}

/** pi 运行转录表：保存一次 agent run 的完整消息流（含工具调用）。 */
export async function ensureSchema(): Promise<void> {
	await pool.execute(`
		CREATE TABLE IF NOT EXISTS agent_transcripts (
			id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
			assistant_message_id BIGINT UNSIGNED NOT NULL,
			owner VARCHAR(100) NOT NULL,
			payload MEDIUMTEXT NOT NULL,
			created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
			KEY idx_agent_transcripts_message (assistant_message_id),
			KEY idx_agent_transcripts_owner (owner)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
	`);
}
