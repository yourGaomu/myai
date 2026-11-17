import { useMemo } from "react";
import { ActivityIndicator, ScrollView, StyleSheet, Text, View } from "react-native";

import type { FileReadResultPayload } from "../../protocol";
import { fontFamily, font, radius } from "../../constants/theme";
import { useAppTheme, useThemedStyles, type AppTheme } from "../../theme/ThemeProvider";
import { useListStyles } from "../common/listStyles";

type Props = {
  loading: boolean;
  preview: FileReadResultPayload | null;
};

const lineHeight = 21;

// 文件预览：行号 + 等宽正文，长行横向滚动；支持深浅主题。
export function FilePreview({ loading, preview }: Props) {
  const { palette } = useAppTheme();
  const list = useListStyles();
  const styles = useThemedStyles(makeStyles);
  const content = (preview?.content || "").replace(/\t/g, "  ");
  const numbers = useMemo(() => {
    const count = content ? content.split("\n").length : 0;
    return Array.from({ length: count }, (_, index) => String(index + 1)).join("\n");
  }, [content]);

  if (!preview || (loading && !content)) {
    return (
      <View style={list.loading}>
        <ActivityIndicator color={palette.ink2} size="small" />
        <Text style={list.loadingText}>正在读取文件…</Text>
      </View>
    );
  }
  if (preview.binary) {
    return <Text style={list.empty}>二进制文件暂不支持预览。</Text>;
  }
  if (!content) {
    return <Text style={list.empty}>文件是空的。</Text>;
  }

  return (
    <View style={styles.box}>
      <Text style={styles.gutter}>{numbers}</Text>
      <ScrollView horizontal showsHorizontalScrollIndicator={false} style={styles.scroll}>
        <Text selectable style={styles.code}>
          {content}
          {preview.truncated ? <Text style={styles.truncated}>{"\n\n… 文件较大，只显示了前一部分"}</Text> : null}
        </Text>
      </ScrollView>
    </View>
  );
}

const makeStyles = ({ palette }: AppTheme) =>
  StyleSheet.create({
    box: {
      backgroundColor: palette.well,
      borderColor: palette.hairline,
      borderRadius: radius.md,
      borderWidth: 1,
      flexDirection: "row",
      paddingVertical: 10,
    },
    gutter: {
      color: palette.ink4,
      fontFamily: fontFamily.mono,
      fontSize: font.micro,
      lineHeight,
      minWidth: 40,
      paddingLeft: 8,
      paddingRight: 10,
      textAlign: "right",
    },
    scroll: { flex: 1 },
    code: {
      color: palette.ink1,
      fontFamily: fontFamily.mono,
      fontSize: font.caption,
      lineHeight,
      paddingRight: 16,
    },
    truncated: { color: palette.ink3 },
  });
