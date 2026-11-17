// 用法：node scripts/migrate-colors.mjs
// 作用：按 3.2/3.3 映射表批量替换 src/ 下的颜色字面量。
// 同时处理两种写法：对象属性 prop: "#hex"  和  JSX 属性 prop="#hex"

import fs from "node:fs";
import path from "node:path";

const SRC = "src";

// ===== 3.2 / 3.3 主映射表 =====
const MAP = {
  "#12100e": { color: "palette.ink1", backgroundColor: "palette.surface3", border: "palette.hairline" },
  "#6c665f": { color: "palette.ink2", backgroundColor: "palette.surface2", border: "palette.hairline" },
  "#ffd84f": { color: "palette.inkInverse", backgroundColor: "palette.brand", border: "palette.brandDeep" },
  "#fffdf7": { color: "palette.ink1", backgroundColor: "palette.surface1", border: "palette.hairline" },
  "#776f66": { color: "palette.ink3", backgroundColor: null, border: "palette.hairlineSoft" },
  "#25231f": { color: "palette.ink1", backgroundColor: "palette.surface4", border: "palette.hairlineStrong" },
  "#fffaf0": { color: "palette.ink1", backgroundColor: "palette.surface1", border: "palette.hairline" },
  "#ffffff": { color: "palette.ink1", backgroundColor: "palette.surface2", border: "palette.hairlineStrong" },
  "#fff": { color: "palette.ink1", backgroundColor: "palette.surface2", border: "palette.hairlineStrong" },
  "#b9e9b0": { color: "palette.inkInverse", backgroundColor: "palette.okWash", border: "palette.okBorder" },
  "#8c857b": { color: "palette.ink3", backgroundColor: "palette.surface3", border: "palette.hairline" },
  "#ff7f68": { color: "palette.inkInverse", backgroundColor: "palette.errWash", border: "palette.errBorder" },
  "#4fd7ee": { color: "palette.inkInverse", backgroundColor: "palette.cyanWash", border: "palette.cyanWash" },
  "#f5eefc": { color: "palette.ink2", backgroundColor: "palette.brandWash", border: "palette.hairline" },
  "#f5f1e9": { color: "palette.ink2", backgroundColor: "palette.surface2", border: "palette.hairline" },
  "#d7cfc2": { color: "palette.ink3", backgroundColor: null, border: "palette.hairline" },
  "#78716c": { color: "palette.ink3", backgroundColor: "palette.surface3", border: "palette.hairline" },
  "#2e8b38": { color: "palette.ok", backgroundColor: "palette.ok", border: "palette.okBorder" },
  "#0f766e": { color: "palette.info", backgroundColor: "palette.infoWash", border: "palette.okBorder" },
  "#ede7da": { color: "palette.ink2", backgroundColor: "palette.surface2", border: "palette.hairline" },
  "#0e4e16": { color: "palette.ok", backgroundColor: "palette.okWash", border: "palette.okBorder" },
  "#777169": { color: "palette.ink3", backgroundColor: null, border: "palette.hairline" },
  "#f4f5f7": { color: "palette.ink1", backgroundColor: "palette.canvas", border: "palette.hairline" },
  "#a8a29e": { color: "palette.ink3", backgroundColor: "palette.surface3", border: "palette.hairlineStrong" },
  "#fff8db": { color: "palette.inkInverse", backgroundColor: "palette.brandWashStrong", border: "palette.brandDeep" },
  "#fee2e2": { color: "palette.err", backgroundColor: "palette.errWash", border: "palette.errBorder" },
  "#44403c": { color: "palette.ink2", backgroundColor: "palette.surface3", border: "palette.hairlineStrong" },
  "#756d64": { color: "palette.ink3", backgroundColor: "palette.surface2", border: "palette.hairline" },
  "#171411": { color: "palette.ink1", backgroundColor: "palette.surface3", border: "palette.hairline" },
  "#141413": { color: "palette.ink1", backgroundColor: "palette.surface1", border: "palette.hairline" },
  // 3.3 浅色底统一归类
  "#dcfce7": { color: "palette.ok", backgroundColor: "palette.okWash", border: "palette.okBorder" },
  "#d9f6df": { color: "palette.ok", backgroundColor: "palette.okWash", border: "palette.okBorder" },
  "#edf5f0": { color: "palette.ok", backgroundColor: "palette.okWash", border: "palette.okBorder" },
  "#dcf5d6": { color: "palette.ok", backgroundColor: "palette.okWash", border: "palette.okBorder" },
  "#e7f3e7": { color: "palette.ok", backgroundColor: "palette.okWash", border: "palette.okBorder" },
  "#edf9e8": { color: "palette.ok", backgroundColor: "palette.okWash", border: "palette.okBorder" },
  "#e6efe4": { color: "palette.ok", backgroundColor: "palette.okWash", border: "palette.okBorder" },
  "#dcebd8": { color: "palette.ok", backgroundColor: "palette.okWash", border: "palette.okBorder" },
  "#dff3dc": { color: "palette.ok", backgroundColor: "palette.okWash", border: "palette.okBorder" },
  "#b9e0c2": { color: "palette.ok", backgroundColor: "palette.okWash", border: "palette.okBorder" },
  "#fef2f2": { color: "palette.err", backgroundColor: "palette.errWash", border: "palette.errBorder" },
  "#ffe1db": { color: "palette.err", backgroundColor: "palette.errWash", border: "palette.errBorder" },
  "#ffe3de": { color: "palette.err", backgroundColor: "palette.errWash", border: "palette.errBorder" },
  "#fff0ee": { color: "palette.err", backgroundColor: "palette.errWash", border: "palette.errBorder" },
  "#eedbd6": { color: "palette.err", backgroundColor: "palette.errWash", border: "palette.errBorder" },
  "#fef3c7": { color: "palette.warn", backgroundColor: "palette.brandWash", border: "palette.brandDeep" },
  "#fff4cc": { color: "palette.warn", backgroundColor: "palette.brandWash", border: "palette.brandDeep" },
  "#fffbeb": { color: "palette.warn", backgroundColor: "palette.brandWash", border: "palette.brandDeep" },
  "#fdf7ea": { color: "palette.warn", backgroundColor: "palette.brandWash", border: "palette.brandDeep" },
  "#cffafe": { color: "palette.cyan", backgroundColor: "palette.cyanWash", border: "palette.cyanWash" },
  "#f0fdfa": { color: "palette.cyan", backgroundColor: "palette.cyanWash", border: "palette.cyanWash" },
  "#eff6ff": { color: "palette.cyan", backgroundColor: "palette.cyanWash", border: "palette.cyanWash" },
  "#eef7ff": { color: "palette.cyan", backgroundColor: "palette.cyanWash", border: "palette.cyanWash" },
  "#f1ece3": { color: "palette.ink2", backgroundColor: "palette.surface2", border: "palette.hairline" },
  "#f7f5f0": { color: "palette.ink2", backgroundColor: "palette.surface2", border: "palette.hairline" },
  "#fdfbf7": { color: "palette.ink1", backgroundColor: "palette.surface1", border: "palette.hairline" },
  "#efe4d2": { color: "palette.ink1", backgroundColor: "palette.canvas", border: "palette.hairline" },
};

// 深色主题下必须删除的硬阴影色（值本身无意义，删整个 shadow 属性）
const SHADOW_KILL = new Set([
  "#12100e", "#25231f", "#ffd84f", "#ffffff", "#fff", "#000", "#141413", "#171411",
]);

const BORDER_PROPS = new Set([
  "borderColor", "borderTopColor", "borderBottomColor",
  "borderLeftColor", "borderRightColor",
]);

// 直接指定目标 token 的属性（不走主表的 color 列）
const DIRECT = {
  placeholderTextColor: "palette.ink4",
  tintColor: "palette.ink2",
};

const TODO = [];

function walk(dir, out = []) {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const f = path.join(dir, e.name);
    if (e.isDirectory()) walk(f, out);
    else if (/\.(tsx?)$/.test(e.name)) out.push(f);
  }
  return out;
}

function resolve(prop, hex) {
  // shadowColor 一律标记删除
  if (prop === "shadowColor") return { kind: "shadow" };

  // 直指属性
  if (DIRECT[prop]) return { kind: "ok", target: DIRECT[prop] };

  const entry = MAP[hex];
  if (!entry) return { kind: "unmapped" };

  let target = null;
  if (prop === "color") target = entry.color;
  else if (prop === "backgroundColor") target = entry.backgroundColor;
  else if (BORDER_PROPS.has(prop)) target = entry.border;
  else return { kind: "unknown-prop" };

  if (!target) return { kind: "null-in-table" };
  return { kind: "ok", target };
}

// 关键：两种写法都匹配
//   1) 对象/内联：  prop: "#hex"      或  prop: '#hex'
//   2) JSX 属性：   prop="#hex"       或  prop='#hex'
const TWO_FORMS = /(\b[A-Za-z][A-Za-z0-9]*)\s*(?::|=)\s*(["'])(#[0-9a-fA-F]{3,8})\2/g;

function migrate(file, src) {
  return src.replace(TWO_FORMS, (full, prop, q, hexRaw) => {
    const hex = hexRaw.toLowerCase();
    const r = resolve(prop, hex);

    if (r.kind === "ok") {
      // 两种写法区别处理：
      //   对象属性  backgroundColor: "#hex"   ->  backgroundColor: palette.xxx
      //   JSX 属性  color="#hex"              ->  color={palette.xxx}   ← 必须加花括号
      const isJsxAttr = full.includes("=") && !full.includes(":");
      return isJsxAttr ? prop + "={" + r.target + "}" : prop + ": " + r.target;
    }
    if (r.kind === "shadow") {
      TODO.push(file + ": shadowColor " + hex + " -> 删除整段 shadow 五件套，改用 shadow.* 展开");
      return full;
    }
    if (r.kind === "unmapped") {
      TODO.push(file + ": 未映射色值 " + hex + " (属性 " + prop + ") -> 按 3.4 兜底规则人工判断");
      return full;
    }
    if (r.kind === "null-in-table" || r.kind === "unknown-prop") {
      TODO.push(file + ": " + prop + " " + hex + " -> 主表该项为空或不适用，人工判断");
      return full;
    }
    return full;
  });
}

function flagLeftovers(file, src) {
  const re = /(["'])(#[0-9a-fA-F]{3,8})\1/g;
  let m;
  while ((m = re.exec(src))) {
    TODO.push(file + ": 游离色值 " + m[2] + " (非 prop:值 / prop=值 形式) -> 人工处理");
  }
}

const files = walk(SRC);
let changed = 0;

for (const file of files) {
  const before = fs.readFileSync(file, "utf8");
  const after = migrate(file, before);
  if (after !== before) {
    fs.writeFileSync(file, after, "utf8");
    changed++;
  }
  flagLeftovers(file, after);
}

const uniq = [...new Set(TODO)];
console.log("\n已修改 " + changed + " 个文件。");
console.log("待人工处理 " + uniq.length + " 项：\n");
for (const t of uniq) console.log("  " + t);
fs.writeFileSync("migration-todo.txt", uniq.join("\n"), "utf8");
console.log("\n清单已写入 mobile/migration-todo.txt");
