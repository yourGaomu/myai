import { ActivityIndicator, Image, Linking, Modal, Pressable, Text, View, type StyleProp, type ViewStyle } from "react-native";
import { useState } from "react";
import { Feather } from "@expo/vector-icons";

import { isPreviewableImageAsset, type SharedAsset } from "../../utils/toolAssets";
import { formatBytes, formatDateTime } from "../../utils/format";
import { useChatStyles } from "./styles";
import { useAppTheme } from "../../theme/ThemeProvider";

type Props = {
  asset: SharedAsset;
  buttonFeedback: (style: StyleProp<ViewStyle>, active?: boolean) => StyleProp<ViewStyle>;
  previewURL?: string;
};

export function SharedAssetCard({ asset, buttonFeedback, previewURL }: Props) {
  const { palette } = useAppTheme();
  const styles = useChatStyles();
  // Public short links can be single-use. Only a picked local file is safe to
  // load automatically; opening a public link always requires a user action.
  const imageSource = previewURL;
  const [imageLoading, setImageLoading] = useState(Boolean(previewURL) && isPreviewableImageAsset(asset));
  const [imageFailed, setImageFailed] = useState(false);
  const [fullScreenVisible, setFullScreenVisible] = useState(false);
  const title = asset.fileName || asset.path || "共享文件";
  const meta = [asset.size !== undefined ? formatBytes(asset.size) : "", asset.contentType || "", asset.expiresAt ? `有效期至 ${formatDateTime(asset.expiresAt)}` : ""]
    .filter(Boolean)
    .join(" / ");
  const showImagePreview = Boolean(imageSource) && isPreviewableImageAsset(asset) && !imageFailed;

  return (
    <View style={[styles.assetCard, showImagePreview && styles.assetImageCard]}>
      {showImagePreview ? (
        <Pressable
          accessibilityLabel="查看大图"
          onPress={() => setFullScreenVisible(true)}
          style={({ pressed }) => buttonFeedback(styles.assetPreviewButton, pressed)}
        >
          <Image
            onError={() => {
              setImageFailed(true);
              setImageLoading(false);
            }}
            onLoadEnd={() => setImageLoading(false)}
            resizeMode="cover"
            source={{ uri: imageSource }}
            style={styles.assetPreviewImage}
          />
          <View style={styles.assetPreviewTapHint}>
            <Feather color={palette.ink1} name="zoom-in" size={16} />
            <Text style={styles.assetPreviewTapHintText}>查看大图</Text>
          </View>
          {imageLoading ? (
            <View style={styles.assetPreviewOverlay}>
              <ActivityIndicator color={palette.ink1} size="small" />
              <Text style={styles.assetPreviewStatus}>正在加载预览</Text>
            </View>
          ) : null}
        </Pressable>
      ) : null}

      {/* 全屏大图沉浸式预览弹窗 */}
      <Modal
        animationType="fade"
        onRequestClose={() => setFullScreenVisible(false)}
        transparent
        visible={fullScreenVisible}
      >
        <View style={styles.imageModalBackdrop}>
          <View style={styles.imageModalHeader}>
            <Text numberOfLines={1} style={styles.imageModalTitle}>
              {title}
            </Text>
            <Pressable
              onPress={() => setFullScreenVisible(false)}
              style={({ pressed }) => buttonFeedback(styles.imageModalCloseBtn, pressed)}
            >
              <Feather color={palette.inkInverse} name="x" size={16} />
              <Text style={styles.imageModalCloseBtnText}>关闭</Text>
            </Pressable>
          </View>

          <Pressable
            onPress={() => setFullScreenVisible(false)}
            style={styles.imageModalContent}
          >
            <Image
              resizeMode="contain"
              source={{ uri: imageSource }}
              style={styles.imageModalFullImage}
            />
          </Pressable>

          {asset.shortURL ? <View style={styles.imageModalFooter}>
            <Pressable
              onPress={() => void Linking.openURL(asset.shortURL!)}
              style={({ pressed }) => buttonFeedback(styles.imageModalActionBtn, pressed)}
            >
              <Feather color={palette.ink1} name="link" size={14} />
              <Text style={styles.imageModalActionBtnText}>浏览器中打开</Text>
            </Pressable>
          </View> : null}
        </View>
      </Modal>
      <View style={styles.assetDetailsRow}>
        <View style={styles.assetIconBox}>
          <Text style={styles.assetIconText}>{showImagePreview ? "图片" : fileInitial(title)}</Text>
        </View>
        <View style={styles.assetContent}>
          <Text numberOfLines={1} style={styles.assetTitle}>
            {title}
          </Text>
          {asset.path ? (
            <Text numberOfLines={1} style={styles.assetPath}>
              {asset.path}
            </Text>
          ) : null}
          {meta ? <Text style={styles.assetMeta}>{meta}</Text> : null}
          {imageFailed ? <Text style={styles.assetPreviewStatus}>无法预览</Text> : null}
          <Text numberOfLines={1} selectable style={styles.assetURL}>
            {asset.shortURL || `已上传 · ${asset.assetID}`}
          </Text>
        </View>
        {asset.shortURL ? <Pressable onPress={() => void Linking.openURL(asset.shortURL!)} style={({ pressed }) => buttonFeedback(styles.assetOpenButton, pressed)}>
          <Text style={styles.assetOpenButtonText}>打开</Text>
        </Pressable> : null}
      </View>
    </View>
  );
}

function fileInitial(name: string) {
  const extension = name.split(".").pop()?.trim();
  if (extension && extension !== name) {
    return extension.slice(0, 3).toUpperCase();
  }
  return "文件";
}
