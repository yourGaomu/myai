import type { ChangeEntry } from "../../protocol";
import { styles } from "./styles";

export function changeBadgeStyle(entry: ChangeEntry) {
  if (entry.deleted) {
    return styles.changeBadgeDeleted;
  }
  if (entry.untracked || entry.status === "added") {
    return styles.changeBadgeAdded;
  }
  if (entry.renamed) {
    return styles.changeBadgeRenamed;
  }
  return styles.changeBadgeModified;
}
