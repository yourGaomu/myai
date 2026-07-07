import { ActivityIndicator, Image, Linking, Pressable, Text, View, type StyleProp, type ViewStyle } from "react-native";
import { useState } from "react";

import { isPreviewableImageAsset, type SharedAsset } from "../../utils/toolAssets";
import { formatBytes, formatDateTime } from "../../utils/format";
import { styles } from "./styles";

type Props = {
  asset: SharedAsset;
  buttonFeedback: (style: StyleProp<ViewStyle>, active?: boolean) => StyleProp<ViewStyle>;
};

export function SharedAssetCard({ asset, buttonFeedback }: Props) {
  const [imageLoading, setImageLoading] = useState(isPreviewableImageAsset(asset));
  const [imageFailed, setImageFailed] = useState(false);
  const title = asset.fileName || asset.path || "Shared file";
  const meta = [asset.size !== undefined ? formatBytes(asset.size) : "", asset.contentType || "", asset.expiresAt ? `expires ${formatDateTime(asset.expiresAt)}` : ""]
    .filter(Boolean)
    .join(" / ");
  const showImagePreview = isPreviewableImageAsset(asset) && !imageFailed;

  return (
    <View style={[styles.assetCard, showImagePreview && styles.assetImageCard]}>
      {showImagePreview ? (
        <Pressable onPress={() => void Linking.openURL(asset.shortURL)} style={({ pressed }) => buttonFeedback(styles.assetPreviewButton, pressed)}>
          <Image
            onError={() => {
              setImageFailed(true);
              setImageLoading(false);
            }}
            onLoadEnd={() => setImageLoading(false)}
            resizeMode="cover"
            source={{ uri: asset.shortURL }}
            style={styles.assetPreviewImage}
          />
          {imageLoading ? (
            <View style={styles.assetPreviewOverlay}>
              <ActivityIndicator color="#12100e" size="small" />
              <Text style={styles.assetPreviewStatus}>Loading preview</Text>
            </View>
          ) : null}
        </Pressable>
      ) : null}
      <View style={styles.assetDetailsRow}>
        <View style={styles.assetIconBox}>
          <Text style={styles.assetIconText}>{showImagePreview ? "IMG" : fileInitial(title)}</Text>
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
          {imageFailed ? <Text style={styles.assetPreviewStatus}>Preview unavailable</Text> : null}
          <Text numberOfLines={1} selectable style={styles.assetURL}>
            {asset.shortURL}
          </Text>
        </View>
        <Pressable onPress={() => void Linking.openURL(asset.shortURL)} style={({ pressed }) => buttonFeedback(styles.assetOpenButton, pressed)}>
          <Text style={styles.assetOpenButtonText}>Open</Text>
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
  return "FILE";
}
