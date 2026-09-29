import katex from "katex";
// Лёгкая подсветка Go для кода внутри вопросов (shiki в браузер не тащим).
const KW = new Set("break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var".split(" "));
const BI = new Set("true false nil iota append cap clear close complex copy delete imag len make max min new panic print println real recover any error string int int8 int16 int32 int64 uint uint8 uint16 uint32 uint64 uintptr byte rune float32 float64 bool comparable".split(" "));

export const esc = (s: string) =>
  String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]!);

const tex = (t: string) => {
  try { return katex.renderToString(t, { output: "html", throwOnError: false }); } catch { return esc(t); }
};
// Текст вопроса: `код`, **жирный** и формулы $...$ (формулы внутри `кода` не трогаем).
export const inline = (s: string) =>
  String(s)
    .split(/(`[^`]+`)/)
    .map((part) => {
      if (/^`[^`]+`$/.test(part)) return `<code>${esc(part.slice(1, -1))}</code>`;
      const chunks = part.split(/(\$[^$\n]+?\$)/);
      return chunks
        .map((c) => (/^\$[^$\n]+\$$/.test(c) ? tex(c.slice(1, -1)) : esc(c).replace(/\*\*([^*]+)\*\*/g, "<b>$1</b>")))
        .join("");
    })
    .join("");

export function hl(src: string): string {
  const re = /(\/\/[^\n]*|\/\*[\s\S]*?\*\/)|("(?:\\.|[^"\\\n])*"|`[^`]*`|'(?:\\.|[^'\\\n])+')|\b(0x[\da-fA-F_]+|\d[\d_]*(?:\.\d+)?(?:e[+-]?\d+)?)\b|\b([A-Za-z_]\w*)\b(\s*\()?/g;
  let out = "", last = 0, m: RegExpExecArray | null;
  while ((m = re.exec(src))) {
    out += esc(src.slice(last, m.index));
    last = re.lastIndex;
    if (m[1]) out += `<span class="tk-com">${esc(m[1])}</span>`;
    else if (m[2]) out += `<span class="tk-str">${esc(m[2])}</span>`;
    else if (m[3]) out += `<span class="tk-num">${esc(m[3])}</span>`;
    else {
      const w = m[4], paren = m[5] || "";
      if (KW.has(w)) out += `<span class="tk-kw">${w}</span>`;
      else if (BI.has(w)) out += `<span class="tk-bi">${w}</span>`;
      else if (paren) out += `<span class="tk-fn">${w}</span>`;
      else out += w;
      out += esc(paren);
    }
  }
  return out + esc(src.slice(last));
}
