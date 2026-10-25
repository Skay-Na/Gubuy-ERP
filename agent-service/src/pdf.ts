// PDF 文本抽取：供聊天附件走 textChunks 路径
// 选用 unpdf（内置现代 pdf.js、ESM 友好、无原生依赖）；
// pdf-parse 捆绑的 pdf.js v1.10.100 在 Node 22 下模块加载期即不稳定，弃用。
import { extractText, getDocumentProxy } from "unpdf";

/** 单附件抽取文本上限（字符），超出部分丢弃并置 truncated 标记 */
export const PDF_TEXT_LIMIT = 20000;

export interface PdfExtractResult {
	text: string;
	truncated: boolean;
}

/** 按 ai_agent_attachments.truncated 的语义返回抽取结果 */
export async function extractPdfText(buffer: Buffer): Promise<PdfExtractResult> {
	const pdf = await getDocumentProxy(new Uint8Array(buffer));
	const { text } = await extractText(pdf, { mergePages: true });
	const merged = Array.isArray(text) ? text.join("\n") : String(text ?? "");
	const full = merged.replace(/\u0000/g, "").trim();
	return {
		text: full.slice(0, PDF_TEXT_LIMIT),
		truncated: full.length > PDF_TEXT_LIMIT,
	};
}
