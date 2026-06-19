import type { ChangeEntry } from "../protocol";

export function changeLabel(entry: ChangeEntry) {
  if (entry.untracked) {
    return "?";
  }
  if (entry.deleted) {
    return "D";
  }
  if (entry.renamed) {
    return "R";
  }
  if (entry.status === "added") {
    return "A";
  }
  if (entry.status === "modified") {
    return "M";
  }
  return "C";
}

export function changeMeta(entry: ChangeEntry) {
  const parts = [entry.status || "changed"];
  if (entry.staged) {
    parts.push("staged");
  }
  if (entry.unstaged) {
    parts.push("unstaged");
  }
  if (entry.old_path) {
    parts.push(`from ${entry.old_path}`);
  }
  return parts.join(" / ");
}
