import { useMemo } from "react";
import { ActivityIndicator, ScrollView, StyleSheet, Text, View } from "react-native";

import type { FileReadResultPayload } from "../../protocol";
import { fontFamily, font, palette, radius } from "../../constants/theme";
import { list } from "../common/listStyles";

type Props = {
  loading: boolean;
  preview: FileReadResultPayload | null;
};

const lineHeight = 21;

// 文件预览：行号 + 等宽正文，长行横向滚动；纵向跟随页面滚动。
export function FilePreview({ loading, preview }: Props) {
  // 制表符在等宽字体里占 8 格，手机上太宽，换成 2 个空格
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

const styles = StyleSheet.create({
  box: {
    backgroundColor: palette.well,
    borderRadius: radius.md,
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
