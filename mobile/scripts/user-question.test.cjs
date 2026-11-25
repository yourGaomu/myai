const assert = require("node:assert/strict");
const fs = require("node:fs");
const { test } = require("node:test");
const ts = require("typescript");
require.extensions[".ts"] = (module, filename) => {
  const { outputText } = ts.transpileModule(fs.readFileSync(filename, "utf8"), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 }, fileName: filename,
  });
  module._compile(outputText, filename);
};
const q = require("../src/utils/userQuestion.ts");
const single = { id: "style", prompt: "Style?", multi_select: false, allow_other: true, choices: [
  { label: "Dark", description: "High contrast", recommended: true }, { label: "Light" },
] };
const multi = { id: "sections", prompt: "Sections?", multi_select: true, allow_other: true, choices: [
  { label: "Hero", recommended: true }, { label: "Pricing", recommended: true }, { label: "Cases" },
] };
test("validates only the required structured schema", () => {
  q.validateQuestionChoices(single);
  assert.throws(() => q.validateQuestionChoices({ ...single, options: ["Dark"] }), /schema/);
  assert.throws(() => q.validateQuestionChoices({ ...single, choices: [{ label: "  " }] }), /invalid choice/);
  assert.throws(() => q.validateQuestionChoices({ ...single, choices: [{ label: "A" }, { label: " A " }] }), /duplicate/);
  assert.throws(() => q.validateQuestionChoices({ ...single, choices: [{ label: "A", recommended: true }, { label: "B", recommended: true }] }), /multiple recommended/);
});
test("single-select preset and custom input are exclusive", () => {
  assert.deepEqual(q.applySingleSelectOption("Dark"), { selected: ["Dark"], other: "" });
  assert.deepEqual(q.applySingleSelectOther("my own"), { selected: [], other: "my own" });
});
test("multi-select allows selections and custom supplement together", () => {
  const selected = q.toggleMultiSelectOption(undefined, "Hero");
  const both = q.updateMultiSelectOther(selected, "Add dark-mode toggle");
  assert.deepEqual(both, { selected: ["Hero"], other: "Add dark-mode toggle" });
  assert.equal(q.formatQuestionAnswerSummary(multi, both), "Hero；补充：Add dark-mode toggle");
});
test("recommended submission fills only unanswered questions", () => {
  const drafts = { style: q.applySingleSelectOther("Brand color is green") };
  const merged = q.mergeWithRecommendedDefaults([single, multi], drafts);
  assert.deepEqual(merged.style, drafts.style);
  assert.deepEqual(merged.sections.selected, ["Hero", "Pricing"]);
  const submission = q.buildUserQuestionSubmission([single, multi], merged, true);
  assert.deepEqual(submission.structuredAnswers.style, { other: "Brand color is green" });
  assert.deepEqual(submission.structuredAnswers.sections, { selected: ["Hero", "Pricing"] });
  assert.equal(submission.adoptedDefaults, true);
  assert.match(q.formatSubmittedCardSummary([single, multi], submission), /其余采用推荐/);
});
test("adopting every recommendation produces only structured data", () => {
  const drafts = q.mergeWithRecommendedDefaults([single, multi], {});
  const submission = q.buildUserQuestionSubmission([single, multi], drafts, true);
  assert.deepEqual(Object.keys(submission).sort(), ["adoptedDefaults", "structuredAnswers"]);
  assert.equal(q.formatSubmittedCardSummary([single, multi], submission), "已按推荐方案提交");
});


test("recommendations are available when every remaining question has a recommendation", () => {
  const manual = { ...single, choices: [{ label: "Light" }] };
  const items = [manual, multi];
  const drafts = { style: q.applySingleSelectOption("Light") };
  assert.equal(q.canAdoptRecommended(items, {}), false);
  assert.equal(q.canAdoptRecommended(items, drafts), true);
  assert.equal(q.canAdoptRecommended(items, q.mergeWithRecommendedDefaults(items, drafts)), false);
});

test("an open question requires custom text to be enabled", () => {
  assert.throws(() => q.validateQuestionChoices({ ...single, choices: [], allow_other: false }), /valid answer options/);
  assert.throws(() => q.buildUserQuestionSubmission([{ ...single, allow_other: false }], {
    style: q.applySingleSelectOther("custom"),
  }), /does not allow custom text/);
});
