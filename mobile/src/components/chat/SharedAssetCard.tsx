import { ActivityIndicator, Image, Linking, Pressable, Text, View, type StyleProp, type ViewStyle } from "react-native";
import { useState } from "react";

import { isPreviewableImageAsset, type SharedAsset } from "../../utils/toolAssets";
import { formatBytes, formatDateTime } from "../../utils/format";
import { styles } from "./styles";

type Props = {
  asset: SharedAsset;
  buttonFeedback: (style: StyleProp<ViewStyle>, active?: boolean) => StyleProp<ViewStyle>;
  previewURL?: string;
};

export function SharedAssetCard({ asset, buttonFeedback, previewURL }: Props) {
  const [imageLoading, setImageLoading] = useState(isPreviewableImageAsset(asset));
  const [imageFailed, setImageFailed] = useState(false);
  const [imageSource, setImageSource] = useState(previewURL || asset.shortURL);
  const title = asset.fileName || asset.path || "共享文件";
  const meta = [asset.size !== undefined ? formatBytes(asset.size) : "", asset.contentType || "", asset.expiresAt ? `有效期至 ${formatDateTime(asset.expiresAt)}` : ""]
    .filter(Boolean)
    .join(" / ");
  const showImagePreview = isPreviewableImageAsset(asset) && !imageFailed;

  return (
    <View style={[styles.assetCard, showImagePreview && styles.assetImageCard]}>
      {showImagePreview ? (
        <Pressable onPress={() => void Linking.openURL(asset.shortURL)} style={({ pressed }) => buttonFeedback(styles.assetPreviewButton, pressed)}>
          <Image
            onError={() => {
              if (previewURL && imageSource === previewURL && previewURL !== asset.shortURL) {
                setImageSource(asset.shortURL);
                setImageLoading(true);
                return;
              }
              setImageFailed(true);
              setImageLoading(false);
            }}
            onLoadEnd={() => setImageLoading(false)}
            resizeMode="cover"
            source={{ uri: imageSource }}
            style={styles.assetPreviewImage}
          />
          {imageLoading ? (
            <View style={styles.assetPreviewOverlay}>
              <ActivityIndicator color="#12100e" size="small" />
              <Text style={styles.assetPreviewStatus}>正在加载预览</Text>
            </View>
          ) : null}
        </Pressable>
      ) : null}
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
            {asset.shortURL}
          </Text>
        </View>
        <Pressable onPress={() => void Linking.openURL(asset.shortURL)} style={({ pressed }) => buttonFeedback(styles.assetOpenButton, pressed)}>
          <Text style={styles.assetOpenButtonText}>打开</Text>
        </Pressable>
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
