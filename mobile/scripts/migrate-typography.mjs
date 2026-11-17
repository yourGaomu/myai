// 用法：node scripts/migrate-typography.mjs
// 作用：把 fontSize / fontWeight / borderWidth / borderRadius 收敛到 token。
// 必须在 migrate-colors.mjs 之后、且颜色已全部 token 化之后运行。

import fs from "node:fs";
import path from "node:path";

const SRC = "src";

// fontSize: 21 档 -> 7 档
const FONT = {
  "8": "font.micro",
  "9": "font.micro",
  "9.5": "font.micro",
  "10": "font.micro",
  "10.5": "font.caption",
  "11": "font.caption",
  "11.5": "font.caption",
  "12": "font.label",
  "12.5": "font.label",
  "13": "font.body",
  "13.5": "font.body",
  "14": "font.body",
  "15": "font.bodyLg",
  "16": "font.title",
  "17": "font.title",
  "18": "font.title",
  "20": "font.display",
  "22": "font.display",
  "24": "font.display",
  "26": "font.display",
  "28": "font.display",
};

// fontWeight: 合法化 + 降档。900/800 一律降 bold（heavy 需人工保留）
const WEIGHT = {
  '"400"': "weight.regular",
  "'400'": "weight.regular",
  '"450"': "weight.regular",   // 非法值，RN 会忽略
  '"500"': "weight.medium",
  '"550"': "weight.medium",    // 非法值
  '"600"': "weight.semibold",
  '"650"': "weight.semibold",  // 非法值
  '"700"': "weight.bold",
  '"800"': "weight.bold",
  '"900"': "weight.bold",      // heavy 需人工挑出
};

// borderWidth
const BORDER_W = {
  "borderWidth: 2": "borderWidth: 1",
  "borderWidth: 2.5": "borderWidth: 1",
  "borderWidth: 3": "borderWidth: 1",
  "borderWidth: 1.5": "borderWidth: 1",
};

// borderRadius
const RADIUS = {
  "0": "0",              // 明确无圆角，保持 0，不引入 token
  "1": "radius.xxs",
  "2": "radius.xxs",
  "3": "radius.xxs",
  "4": "radius.xs",
  "5": "radius.xs",
  "6": "radius.sm",
  "7": "radius.sm",
  "8": "radius.sm",
  "9": "radius.sm",
  "10": "radius.md",
  "12": "radius.md",
  "13": "radius.md",
  "14": "radius.md",
  "16": "radius.lg",
  "18": "radius.lg",
  "20": "radius.xl",
  "22": "radius.xl",
  "24": "radius.xl",
  "34": "radius.pill",   // 常用于「高度/2」的正圆
  "999": "radius.pill",
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

function migrate(file, src) {
  let out = src;

  // fontSize: 13.5  ->  fontSize: font.body
  out = out.replace(/fontSize:\s*([\d.]+)/g, (full, v) => {
    const t = FONT[v];
    if (!t) {
      TODO.push(file + ": fontSize " + v + " 无映射，人工判断");
      return full;
    }
    return "fontSize: " + t;
  });

  // fontWeight: "900" -> fontWeight: weight.bold
  out = out.replace(/fontWeight:\s*("(?:400|450|500|550|600|650|700|800|900)"|'(?:400|500|600|700|800|900)')/g, (full, v) => {
    const t = WEIGHT[v];
    if (!t) {
      TODO.push(file + ": fontWeight " + v + " 无映射，人工判断");
      return full;
    }
    return "fontWeight: " + t;
  });

  // borderWidth
  for (const [from, to] of Object.entries(BORDER_W)) {
    const re = new RegExp(from.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"), "g");
    out = out.replace(re, to);
  }

  // borderRadius: 14 -> borderRadius: radius.md
  out = out.replace(/\bborderRadius:\s*(\d+)(?!\d)/g, (full, v) => {
    const t = RADIUS[v];
    if (!t) {
      TODO.push(file + ": borderRadius " + v + " 无映射，人工判断");
      return full;
    }
    return "borderRadius: " + t;
  });

  // 子圆角属性同样处理
  for (const prop of ["borderTopLeftRadius", "borderTopRightRadius", "borderBottomLeftRadius", "borderBottomRightRadius", "borderTopStartRadius", "borderTopEndRadius", "borderBottomStartRadius", "borderBottomEndRadius"]) {
    const re = new RegExp("\\b" + prop + ":\\s*(\\d+)(?!\\d)", "g");
    out = out.replace(re, (full, v) => {
      const t = RADIUS[v];
      if (!t) {
        TODO.push(file + ": " + prop + " " + v + " 无映射，人工判断");
        return full;
      }
      return prop + ": " + t;
    });
  }

  return out;
}

const files = walk(SRC);
let changed = 0;

for (const f of files) {
  if (f.endsWith("theme.ts")) continue;   // 跳过 token 文件本身
  const before = fs.readFileSync(f, "utf8");
  const after = migrate(f, before);
  if (after !== before) {
    fs.writeFileSync(f, after, "utf8");
    changed++;
  }
}

const uniq = [...new Set(TODO)];
console.log("\n已修改 " + changed + " 个文件。");
if (uniq.length) {
  console.log("待人工处理 " + uniq.length + " 项：");
  for (const t of uniq) console.log("  " + t);
  fs.writeFileSync("typography-todo.txt", uniq.join("\n"), "utf8");
} else {
  console.log("无待办项。");
}
console.log("\n提醒：本脚本会把所有 fontWeight 900 降为 bold。");
console.log("请人工复核，把『页面主标题 / 数字徽章 / 进度数字』三处改回 weight.heavy。");
