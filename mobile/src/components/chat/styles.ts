import { Platform, StyleSheet } from "react-native";

export const styles = StyleSheet.create({
  flex: {
    flex: 1,
  },
  message: {
    borderRadius: 8,
    borderWidth: 3,
    padding: 10,
  },
  userMessage: {
    alignSelf: "flex-end",
    backgroundColor: "#b9e9b0",
    borderColor: "#12100e",
    maxWidth: "92%",
  },
  assistantMessage: {
    alignSelf: "stretch",
    backgroundColor: "#fffaf0",
    borderColor: "#12100e",
  },
  assistantLoadingMessage: {
    alignItems: "center",
    flexDirection: "row",
    gap: 8,
  },
  assistantLoadingText: {
    color: "#12100e",
    fontWeight: "900",
  },
  eventMessage: {
    alignSelf: "stretch",
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
  },
  toolMessage: {
    alignSelf: "stretch",
    backgroundColor: "#4fd7ee",
    borderColor: "#12100e",
    gap: 8,
  },
  errorMessage: {
    alignSelf: "stretch",
    backgroundColor: "#ff7f68",
    borderColor: "#12100e",
  },
  messageText: {
    color: "#12100e",
    lineHeight: 20,
  },
  messageMeta: {
    color: "#6c665f",
    fontSize: 11,
    marginTop: 8,
  },
  messageStatusRow: {
    alignItems: "center",
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
    marginTop: 8,
  },
  messageStatusPill: {
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
    borderRadius: 6,
    borderWidth: 2,
    color: "#12100e",
    fontSize: 10,
    fontWeight: "900",
    overflow: "hidden",
    paddingHorizontal: 7,
    paddingVertical: 4,
    textTransform: "uppercase",
  },
  regenerateButton: {
    backgroundColor: "#4fd7ee",
    borderColor: "#12100e",
    borderRadius: 6,
    borderWidth: 2,
    paddingHorizontal: 9,
    paddingVertical: 5,
  },
  regenerateButtonText: {
    color: "#12100e",
    fontSize: 11,
    fontWeight: "900",
  },
  reasoningBox: {
    backgroundColor: "#f5eefc",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 3,
    gap: 5,
    marginBottom: 8,
    padding: 9,
  },
  reasoningTitle: {
    color: "#12100e",
    fontSize: 11,
    fontWeight: "900",
  },
  reasoningText: {
    color: "#12100e",
    fontSize: 12,
    lineHeight: 18,
  },
  toolHeader: {
    alignItems: "center",
    flexDirection: "row",
    gap: 10,
  },
  toolBadge: {
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
    borderRadius: 6,
    borderWidth: 2,
    color: "#12100e",
    fontSize: 10,
    fontWeight: "900",
    overflow: "hidden",
    paddingHorizontal: 7,
    paddingVertical: 4,
    textAlign: "center",
    width: 44,
  },
  toolTitle: {
    color: "#12100e",
    fontSize: 13,
    fontWeight: "900",
  },
  toolSubtitle: {
    color: "#12100e",
    fontSize: 11,
    marginTop: 2,
  },
  toolToggle: {
    color: "#12100e",
    fontSize: 12,
    fontWeight: "900",
  },
  toolBody: {
    borderTopColor: "#12100e",
    borderTopWidth: 3,
    gap: 8,
    paddingTop: 8,
  },
  toolSection: {
    gap: 5,
  },
  toolSectionTitle: {
    color: "#12100e",
    fontSize: 11,
    fontWeight: "900",
  },
  toolCode: {
    color: "#12100e",
    fontFamily: Platform.select({ ios: "Menlo", android: "monospace", default: "monospace" }),
    fontSize: 12,
    lineHeight: 18,
  },
  toolErrorText: {
    color: "#7a1f1a",
  },
  markdownRoot: {
    gap: 8,
  },
  markdownHeading: {
    color: "#12100e",
    fontWeight: "900",
  },
  markdownHeading1: {
    fontSize: 20,
    lineHeight: 26,
  },
  markdownHeading2: {
    fontSize: 17,
    lineHeight: 23,
  },
  markdownHeading3: {
    fontSize: 15,
    lineHeight: 21,
  },
  markdownParagraph: {
    color: "#12100e",
    lineHeight: 20,
  },
  markdownStrong: {
    fontWeight: "900",
  },
  markdownEmphasis: {
    fontStyle: "italic",
  },
  markdownInlineCode: {
    backgroundColor: "#f5eefc",
    borderColor: "#12100e",
    borderRadius: 4,
    borderWidth: 1,
    fontFamily: Platform.select({ ios: "Menlo", android: "monospace", default: "monospace" }),
    paddingHorizontal: 4,
  },
  markdownList: {
    gap: 6,
  },
  markdownListItem: {
    flexDirection: "row",
    gap: 8,
  },
  markdownListMarker: {
    color: "#12100e",
    fontWeight: "900",
    width: 20,
  },
  markdownListText: {
    color: "#12100e",
    flex: 1,
    lineHeight: 20,
  },
  markdownCodeBlock: {
    backgroundColor: "#f1ece3",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    gap: 6,
    padding: 8,
  },
  markdownCodeLabel: {
    color: "#6c665f",
    fontSize: 10,
    fontWeight: "900",
    textTransform: "uppercase",
  },
  markdownCodeScroll: {
    maxWidth: "100%",
  },
  markdownCodeText: {
    color: "#12100e",
    fontFamily: Platform.select({ ios: "Menlo", android: "monospace", default: "monospace" }),
    fontSize: 12,
    lineHeight: 18,
  },
  markdownTableScroll: {
    maxWidth: "100%",
  },
  markdownTable: {
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    overflow: "hidden",
  },
  markdownTableHeaderRow: {
    flexDirection: "row",
  },
  markdownTableRow: {
    flexDirection: "row",
  },
  markdownTableRowAlt: {
    backgroundColor: "#f5f1e9",
  },
  markdownTableCell: {
    borderColor: "#12100e",
    borderRightWidth: 1,
    borderBottomWidth: 1,
    minWidth: 120,
    paddingHorizontal: 10,
    paddingVertical: 7,
  },
  markdownTableHeaderCell: {
    backgroundColor: "#ffd84f",
  },
  markdownTableCellText: {
    color: "#12100e",
    fontSize: 11,
    lineHeight: 17,
  },
  markdownQuote: {
    alignItems: "stretch",
    backgroundColor: "#f5f1e9",
    borderColor: "#12100e",
    borderRadius: 8,
    borderWidth: 2,
    flexDirection: "row",
    gap: 8,
    padding: 8,
  },
  markdownQuoteBar: {
    backgroundColor: "#4fd7ee",
    borderRadius: 3,
    width: 5,
  },
  markdownQuoteText: {
    color: "#12100e",
    lineHeight: 20,
  },
});
