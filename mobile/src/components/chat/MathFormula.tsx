import { createElement } from "react";
import { Platform, ScrollView, StyleSheet, Text, useWindowDimensions, View, type TextStyle } from "react-native";
import { SvgXml } from "react-native-svg";

import { useAppTheme } from "../../theme/ThemeProvider";
import { renderMathDrawing } from "../../utils/mathDrawing";

export function MathFormula({ text, source, display = false, textStyle }: {
  text: string;
  source: string;
  display?: boolean;
  textStyle?: TextStyle;
}) {
  const { palette, readingFontSize } = useAppTheme();
  const { fontScale, width: viewportWidth } = useWindowDimensions();
  const drawing = renderMathDrawing(text, display);
  const fallback = <Text selectable style={display ? textStyle : undefined}>{source}</Text>;
  if (!drawing) return fallback;

  const em = (textStyle?.fontSize ?? readingFontSize) * (Platform.OS === "web" ? 1 : fontScale);
  const naturalWidth = drawing.widthEm * em;
  // Keep an unusually long inline formula within narrow chat columns.
  const scale = display ? 1 : Math.min(1, Math.max(100, viewportWidth - 80) / naturalWidth);
  const width = Math.max(1, naturalWidth * scale);
  const height = Math.max(1, drawing.heightEm * em * scale);
  const web = Platform.OS === "web";
  const color = textStyle?.color ?? palette.ink1;
  const svg = (
    <SvgXml
      {...(web ? { "aria-label": `数学公式：${text}` } : { accessibilityLabel: `数学公式：${text}`, accessibilityRole: "image" as const, accessible: true })}
      color={color}
      fallback={fallback}
      height={height}
      width={width}
      xml={drawing.xml}
    />
  );

  if (display) {
    return <ScrollView horizontal contentContainerStyle={styles.display} style={styles.scroll}>{svg}</ScrollView>;
  }
  // Web uses the TeX descent for baseline alignment. Native Text measures a fixed-size attachment.
  if (web) {
    return createElement("span", { style: { display: "inline-block", width, height, verticalAlign: -drawing.depthEm * em * scale } }, svg);
  }
  return <View style={{ height, width }}>{svg}</View>;
}

const styles = StyleSheet.create({
  scroll: { maxWidth: "100%" },
  display: { flexGrow: 1, justifyContent: "center", paddingVertical: 4 },
});
