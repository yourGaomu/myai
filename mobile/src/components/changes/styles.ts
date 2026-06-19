import { StyleSheet } from "react-native";

export const styles = StyleSheet.create({
  panel: {
    backgroundColor: "#fffaf0",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 4,
    elevation: 2,
    gap: 10,
    padding: 12,
    shadowColor: "#12100e",
    shadowOffset: { width: 4, height: 4 },
    shadowOpacity: 0.12,
    shadowRadius: 0,
  },
  changesPanel: {
    minHeight: 320,
  },
  panelHeader: {
    alignItems: "center",
    flexDirection: "row",
    gap: 10,
    justifyContent: "space-between",
  },
  panelTitle: {
    color: "#12100e",
    fontSize: 16,
    fontWeight: "900",
  },
  pathText: {
    color: "#6c665f",
    fontSize: 12,
    fontWeight: "700",
    marginTop: 3,
  },
  rowCompact: {
    flexDirection: "row",
    gap: 8,
  },
  flex: {
    flex: 1,
    minWidth: 0,
  },
  smallButton: {
    backgroundColor: "#f5eefc",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    paddingHorizontal: 10,
    paddingVertical: 7,
  },
  disabledButton: {
    opacity: 0.45,
  },
  changeList: {
    gap: 8,
  },
  changeRow: {
    alignItems: "center",
    backgroundColor: "#f5eefc",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    flexDirection: "row",
    gap: 10,
    padding: 10,
  },
  changeRowActive: {
    backgroundColor: "#b9e9b0",
  },
  changeBadge: {
    borderRadius: 6,
    color: "#12100e",
    fontSize: 12,
    fontWeight: "900",
    overflow: "hidden",
    paddingHorizontal: 8,
    paddingVertical: 4,
    textAlign: "center",
    width: 30,
  },
  changeBadgeModified: {
    backgroundColor: "#4fd7ee",
  },
  changeBadgeAdded: {
    backgroundColor: "#b9e9b0",
  },
  changeBadgeDeleted: {
    backgroundColor: "#ff7f68",
  },
  changeBadgeRenamed: {
    backgroundColor: "#ffd84f",
  },
  fileName: {
    color: "#12100e",
    fontWeight: "900",
  },
  fileMeta: {
    color: "#6c665f",
    fontSize: 12,
    marginTop: 2,
  },
  historyBox: {
    backgroundColor: "#fdf7ea",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    gap: 8,
    padding: 12,
  },
  historyRow: {
    alignItems: "center",
    backgroundColor: "#f5eefc",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    flexDirection: "row",
    gap: 10,
    padding: 10,
  },
  previewHeader: {
    alignItems: "center",
    flexDirection: "row",
    gap: 8,
  },
  previewTitle: {
    color: "#12100e",
    fontSize: 12,
    fontWeight: "900",
  },
  previewButton: {
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    paddingHorizontal: 10,
    paddingVertical: 7,
  },
  emptyText: {
    color: "#6c665f",
  },
  diffDetailPanel: {
    gap: 12,
  },
  detailHeader: {
    alignItems: "center",
    flexDirection: "row",
    gap: 10,
  },
  detailBackButton: {
    alignItems: "center",
    backgroundColor: "#4fd7ee",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    justifyContent: "center",
    minHeight: 44,
    minWidth: 62,
    paddingHorizontal: 12,
  },
  detailBackText: {
    color: "#12100e",
    fontSize: 12,
    fontWeight: "900",
  },
  diffDetailContent: {
    gap: 10,
  },
  detailActions: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
  },
  historyDiffFile: {
    backgroundColor: "#f5eefc",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    gap: 6,
    padding: 10,
  },
});
