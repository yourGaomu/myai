export type DiffLineKind = "add" | "remove" | "context" | "hunk" | "meta";

export type DiffCell = {
  kind: DiffLineKind;
  lineNumber?: number;
  text: string;
};

export type DiffRow =
  | {
      id: string;
      type: "full";
      cell: DiffCell;
    }
  | {
      id: string;
      type: "split";
      before?: DiffCell;
      after?: DiffCell;
    };

export function parseDiffRows(diff: string): DiffRow[] {
  const lines = diff.replace(/\r\n/g, "\n").split("\n");
  const rows: DiffRow[] = [];
  const pendingRemoves: DiffCell[] = [];
  let lineCounter = 0;
  let oldLineNumber = 0;
  let newLineNumber = 0;

  const flushPendingRemove = () => {
    while (pendingRemoves.length > 0) {
      rows.push({
        id: `r-${lineCounter++}`,
        type: "split",
        before: pendingRemoves.shift(),
        after: undefined,
      });
    }
  };

  for (const line of lines) {
    if (!line) {
      flushPendingRemove();
      continue;
    }

    if (line.startsWith("@@")) {
      flushPendingRemove();
      const match = line.match(/@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/);
      if (match) {
        oldLineNumber = Number(match[1]);
        newLineNumber = Number(match[2]);
      }
      rows.push({
        id: `h-${lineCounter++}`,
        type: "full",
        cell: { kind: "hunk", text: line },
      });
      continue;
    }

    if (line.startsWith("diff ") || line.startsWith("index ") || line.startsWith("--- ") || line.startsWith("+++ ")) {
      flushPendingRemove();
      rows.push({
        id: `m-${lineCounter++}`,
        type: "full",
        cell: { kind: "meta", text: line },
      });
      continue;
    }

    if (line.startsWith("-") && !line.startsWith("---")) {
      pendingRemoves.push({ kind: "remove", lineNumber: oldLineNumber++, text: line.slice(1) });
      continue;
    }

    if (line.startsWith("+") && !line.startsWith("+++")) {
      const addCell: DiffCell = { kind: "add", lineNumber: newLineNumber++, text: line.slice(1) };
      if (pendingRemoves.length > 0) {
        rows.push({
          id: `c-${lineCounter++}`,
          type: "split",
          before: pendingRemoves.shift(),
          after: addCell,
        });
      } else {
        rows.push({
          id: `a-${lineCounter++}`,
          type: "split",
          before: undefined,
          after: addCell,
        });
      }
      continue;
    }

    flushPendingRemove();
    if (line.startsWith(" ")) {
      const text = line.slice(1);
      rows.push({
        id: `ctx-${lineCounter++}`,
        type: "split",
        before: { kind: "context", lineNumber: oldLineNumber++, text },
        after: { kind: "context", lineNumber: newLineNumber++, text },
      });
      continue;
    }

    rows.push({
      id: `t-${lineCounter++}`,
      type: "full",
      cell: { kind: "context", text: line },
    });
  }

  flushPendingRemove();
  return rows;
}

export function formatDiffLine(cell: DiffCell) {
  if (cell.lineNumber !== undefined) {
    return `${cell.lineNumber.toString().padStart(4, " ")}  ${cell.text}`;
  }
  return cell.text || " ";
}

