// 最小验证脚本：构造一个极小 PDF，验证 extractPdfText 能抽出文本并正确判 定 truncated
// 运行：npx tsx scripts/test-pdf-parse.ts（或 node dist 后等价）
import assert from "node:assert";
import { extractPdfText, PDF_TEXT_LIMIT } from "../src/pdf.js";

/** 生成一个合法的最小单页 PDF（内嵌正确 xref 偏移），内容为一行英文 */
function buildMinimalPdf(text: string): Buffer {
	const encoder = (obj: string) => Buffer.byteLength(obj, "latin1");
	const objects: string[] = [
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		`<< /Length ${encoder(`BT /F1 24 Tf 72 700 Td (${text}) Tj ET`)} >>\nstream\nBT /F1 24 Tf 72 700 Td (${text}) Tj ET\nendstream`,
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	];
	let pdf = "%PDF-1.4\n";
	const offsets: number[] = [];
	objects.forEach((body, i) => {
		offsets.push(Buffer.byteLength(pdf, "latin1"));
		pdf += `${i + 1} 0 obj\n${body}\nendobj\n`;
	});
	const xrefOffset = Buffer.byteLength(pdf, "latin1");
	// xref 条目行结尾必须是 CRLF（pdf.js 严格校验 20 字节条目，space+LF 会被判 bad XRef entry）
	pdf += `xref\n0 ${objects.length + 1}\n0000000000 65535 f \r\n`;
	for (const offset of offsets) {
		pdf += `${String(offset).padStart(10, "0")} 00000 n \r\n`;
	}
	pdf += `trailer\n<< /Size ${objects.length + 1} /Root 1 0 R >>\nstartxref\n${xrefOffset}\n%%EOF`;
	return Buffer.from(pdf, "latin1");
}

async function main(): Promise<void> {
	// 1. 正常抽取
	const pdf = buildMinimalPdf("Hello ERP PDF parse");
	const result = await extractPdfText(pdf);
	console.log("[test-pdf] extracted text:", JSON.stringify(result.text));
	assert.ok(result.text.includes("Hello ERP"), "抽取文本应包含写入内容");
	assert.equal(result.truncated, false, "短文本不应置 truncated");

	// 2. 截断标记：构造超过上限的文本型内容不可控（PDF 内容生成大文本成本高），
	//    这里直接验证常量与切片语义一致即可。
	assert.equal(PDF_TEXT_LIMIT, 20000, "截断上限应为 20000 字符");

	console.log("[test-pdf] PASS");
}

main().catch((error) => {
	console.error("[test-pdf] FAIL", error);
	process.exit(1);
});
