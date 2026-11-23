import { LiteElement } from "mathjax-full/js/adaptors/lite/Element.js";
import { liteAdaptor } from "mathjax-full/js/adaptors/liteAdaptor.js";
import { RegisterHTMLHandler } from "mathjax-full/js/handlers/html.js";
import { TeX } from "mathjax-full/js/input/tex.js";
import "mathjax-full/js/input/tex/ams/AmsConfiguration.js";
import { mathjax } from "mathjax-full/js/mathjax.js";
import { SVG } from "mathjax-full/js/output/svg.js";

const maxFormulaLength = 2048;
const maxSvgLength = 160_000;
const maxCacheEntries = 128;
const maxCacheCharacters = 1_000_000;
const unitsPerEm = 1000;

export type MathDrawing = {
  xml: string;
  widthEm: number;
  heightEm: number;
  depthEm: number;
};

function createEngine() {
  const adaptor = liteAdaptor();
  RegisterHTMLHandler(adaptor);
  const input = new TeX({
    packages: ["base", "ams"],
    maxBuffer: 4096,
    maxMacros: 500,
    formatError: (_input: unknown, error: Error) => { throw error; },
  });
  const document = mathjax.document("", { InputJax: input, OutputJax: new SVG({ fontCache: "none" }) });
  return { adaptor, document, input };
}

let engine: ReturnType<typeof createEngine> | undefined;
const drawings = new Map<string, MathDrawing | null>();
let cacheCharacters = 0;

export function renderMathDrawing(text: string, display: boolean): MathDrawing | null {
  if (!text.trim() || text.length > maxFormulaLength) return null;
  const key = `${display ? "display" : "inline"}:${text}`;
  const cached = drawings.get(key);
  if (cached !== undefined) {
    drawings.delete(key);
    drawings.set(key, cached);
    return cached;
  }
  let drawing: MathDrawing | null = null;
  try {
    const { adaptor, document, input } = engine ??= createEngine();
    input.reset();
    const svg = adaptor.firstChild(document.convert(text, { display }));
    if (svg instanceof LiteElement && adaptor.kind(svg) === "svg") {
      const [, top, width, height] = adaptor.getAttribute(svg, "viewBox").split(/\s+/).map(Number);
      if (Number.isFinite(top) && width > 0 && width <= 1_000_000 && height > 0 && height <= 100_000) {
        adaptor.removeAttribute(svg, "style");
        adaptor.removeAttribute(svg, "width");
        adaptor.removeAttribute(svg, "height");
        const xml = adaptor.outerHTML(svg);
        if (xml.length <= maxSvgLength && !/<(?:a|image|script|foreignObject)\b/i.test(xml)) {
          drawing = {
            xml,
            widthEm: width / unitsPerEm,
            heightEm: height / unitsPerEm,
            depthEm: Math.max(0, top + height) / unitsPerEm,
          };
        }
      }
    }
  } catch {
    // Keep the original source visible if the engine cannot typeset it.
  }
  drawings.set(key, drawing);
  cacheCharacters += key.length + (drawing?.xml.length ?? 0);
  while (drawings.size > maxCacheEntries || cacheCharacters > maxCacheCharacters) {
    const oldest = drawings.entries().next().value;
    if (!oldest) break;
    const [oldKey, oldDrawing] = oldest;
    cacheCharacters -= oldKey.length + (oldDrawing?.xml.length ?? 0);
    drawings.delete(oldKey);
  }
  return drawing;
}
