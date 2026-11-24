import { useContext, useEffect, useState } from "react";
import { ActivityIndicator, Linking, Modal, Pressable, Text, View } from "react-native";
import { Feather } from "@expo/vector-icons";
import { Image } from "expo-image";

import { AssetPreviewConnectionContext, AssetPreviewVisibleContext } from "../../hooks/useAssetImagePreview";
import { useAppTheme } from "../../theme/ThemeProvider";
import type { ButtonFeedback } from "../../types/ui";
import { isAssetID, pickedImageURI, type AssetImagePreviewError, type AssetImagePreviewLoader, type AssetImagePreviewResult } from "../../utils/assetImagePreview";
import { formatBytes, formatDateTime } from "../../utils/format";
import { assetPublicURL, isPreviewableImageAsset, isRelayPreviewImageAsset, type SharedAsset } from "../../utils/toolAssets";
import { useChatStyles } from "./styles";

type Props = {
  asset: SharedAsset;
  buttonFeedback: ButtonFeedback;
  sessionID?: string;
  localURI?: string;
  imageReference?: boolean;
};

type PreviewProps = {
  assetID: string;
  buttonFeedback: ButtonFeedback;
  sessionID?: string;
  title: string;
};

export function SharedAssetCard({ asset, buttonFeedback, imageReference, localURI, sessionID }: Props) {
  const styles = useChatStyles();
  const visible = useContext(AssetPreviewVisibleContext);
  const local = isPreviewableImageAsset(asset) ? pickedImageURI(localURI) : undefined;
  const canFetch = Boolean(sessionID && isAssetID(asset.assetID) && (imageReference || isRelayPreviewImageAsset(asset)));
  const previewable = Boolean(local || canFetch);
  const shortURL = assetPublicURL(asset.shortURL);
  const title = asset.fileName || asset.path || (imageReference ? "图片" : "共享文件");
  const meta = [asset.size !== undefined ? formatBytes(asset.size) : "", asset.contentType || "", asset.expiresAt ? `有效期至 ${formatDateTime(asset.expiresAt)}` : ""]
    .filter(Boolean)
    .join(" / ");

  return (
    <View style={[styles.assetCard, previewable && styles.assetImageCard]}>
      {previewable ? visible ? (
        <AssetPreview
          assetID={asset.assetID}
          buttonFeedback={buttonFeedback}
          key={JSON.stringify([sessionID, asset.assetID, local, asset.checksum])}
          localURI={local}
          sessionID={canFetch ? sessionID : undefined}
          title={title}
        />
      ) : <View style={[styles.assetPreviewButton, styles.assetPreviewFrame]} /> : null}
      <View style={styles.assetDetailsRow}>
        <View style={styles.assetIconBox}>
          <Text style={styles.assetIconText}>{previewable ? "图片" : fileInitial(title)}</Text>
        </View>
        <View style={styles.assetContent}>
          <Text numberOfLines={1} style={styles.assetTitle}>{title}</Text>
          {asset.path ? <Text numberOfLines={1} style={styles.assetPath}>{asset.path}</Text> : null}
          {meta ? <Text style={styles.assetMeta}>{meta}</Text> : null}
          <Text numberOfLines={1} selectable style={styles.assetURL}>{shortURL || asset.assetID}</Text>
        </View>
        {shortURL ? (
          <Pressable accessibilityRole="link" onPress={() => void Linking.openURL(shortURL).catch(() => {})} style={({ pressed }) => buttonFeedback(styles.assetOpenButton, pressed)}>
            <Text style={styles.assetOpenButtonText}>打开分享链接</Text>
          </Pressable>
        ) : null}
      </View>
    </View>
  );
}

function AssetPreview({ localURI, ...props }: PreviewProps & { localURI?: string }) {
  const [localFailed, setLocalFailed] = useState(false);
  const connection = useContext(AssetPreviewConnectionContext);
  if (localURI && !localFailed) {
    return <PreviewImage buttonFeedback={props.buttonFeedback} onError={() => setLocalFailed(true)} title={props.title} uri={localURI} />;
  }
  if (!props.sessionID || !connection?.connectionID) {
    return <PreviewPlaceholder reason={props.sessionID ? "offline" : "invalid"} onRetry={localURI ? () => setLocalFailed(false) : undefined} />;
  }
  return <RemoteAssetPreview key={connection.connectionID} {...props} sessionID={props.sessionID} loader={connection.loader} />;
}

function RemoteAssetPreview({ assetID, buttonFeedback, loader, sessionID, title }: Omit<PreviewProps, "sessionID"> & {
  loader: AssetImagePreviewLoader;
  sessionID: string;
}) {
  const [result, setResult] = useState<AssetImagePreviewResult | null>(null);
  const [attempt, setAttempt] = useState(0);
  useEffect(() => loader.subscribe(sessionID, assetID, setResult), [assetID, attempt, loader, sessionID]);

  if (result?.status === "ready") {
    const image = result.image;
    return (
      <PreviewImage
        buttonFeedback={buttonFeedback}
        height={image.height}
        onError={() => {
          loader.discard(sessionID, assetID, image);
          setResult((current) => current === result ? { status: "error", reason: "invalid" } : current);
        }}
        title={title}
        uri={image.uri}
        width={image.width}
      />
    );
  }
  return <PreviewPlaceholder reason={result?.reason} onRetry={() => {
    setResult(null);
    setAttempt((value) => value + 1);
  }} />;
}

const previewFailureMessage: Record<AssetImagePreviewError, string> = {
  offline: "中继未连接，连接后可预览",
  timeout: "预览超时，请检查中继或版本后重试",
  unsupported: "当前中继或 Agent 不支持私有预览",
  unavailable: "此图片暂无法通过中继预览",
  invalid: "无法读取图片预览",
  busy: "预览请求较多，请稍后重试",
};

function PreviewPlaceholder({ reason, onRetry }: { reason?: AssetImagePreviewError; onRetry?: () => void }) {
  const styles = useChatStyles();
  const { palette } = useAppTheme();
  return (
    <View style={[styles.assetPreviewButton, styles.assetPreviewFrame, styles.assetPreviewPlaceholder]}>
      {reason ? <Feather color={palette.ink3} name="image" size={20} /> : <ActivityIndicator color={palette.ink2} size="small" />}
      <Text accessibilityLiveRegion="polite" style={styles.assetPreviewStatus}>{reason ? previewFailureMessage[reason] : "正在加载预览"}</Text>
      {reason && onRetry ? (
        <Pressable accessibilityRole="button" hitSlop={6} onPress={onRetry}>
          <Text style={styles.assetOpenButtonText}>重试预览</Text>
        </Pressable>
      ) : null}
    </View>
  );
}

function PreviewImage({ buttonFeedback, height, onError, title, uri, width }: {
  buttonFeedback: ButtonFeedback;
  height?: number;
  onError: () => void;
  title: string;
  uri: string;
  width?: number;
}) {
  const { palette } = useAppTheme();
  const styles = useChatStyles();
  const [loading, setLoading] = useState(true);
  const [fullScreen, setFullScreen] = useState(false);
  const hasDimensions = Boolean(width && height && width > 0 && height > 0);
  const rawRatio = hasDimensions ? width! / height! : undefined;
  const clampedRatio = rawRatio !== undefined ? Math.max(0.78, Math.min(1.85, rawRatio)) : undefined;
  const useContain = rawRatio !== undefined && clampedRatio !== undefined && Math.abs(rawRatio - clampedRatio) > 0.08;
  return (
    <>
      <Pressable
        accessibilityLabel={`查看大图：${title}`}
        accessibilityRole="button"
        disabled={loading}
        onPress={() => setFullScreen(true)}
        style={({ pressed }) => buttonFeedback([
          styles.assetPreviewButton,
          styles.assetPreviewFrame,
          clampedRatio !== undefined ? { aspectRatio: clampedRatio } : null,
        ], pressed)}
      >
        <Image
          cachePolicy="memory"
          contentFit={useContain ? "contain" : "cover"}
          onError={onError}
          onLoad={() => setLoading(false)}
          source={{ uri }}
          style={styles.assetPreviewImage}
        />
        {loading ? (
          <View style={styles.assetPreviewOverlay}>
            <ActivityIndicator color={palette.ink1} size="small" />
            <Text style={styles.assetPreviewStatus}>正在加载预览</Text>
          </View>
        ) : (
          <View style={styles.assetPreviewTapHint}>
            <Feather color={palette.ink1} name="zoom-in" size={16} />
            <Text style={styles.assetPreviewTapHintText}>查看大图</Text>
          </View>
        )}
      </Pressable>
      {fullScreen ? (
        <Modal animationType="fade" onRequestClose={() => setFullScreen(false)} transparent visible>
          <View style={styles.imageModalBackdrop}>
            <View style={styles.imageModalHeader}>
              <Text numberOfLines={1} style={styles.imageModalTitle}>{title}</Text>
              <Pressable accessibilityLabel="关闭大图" accessibilityRole="button" onPress={() => setFullScreen(false)} style={({ pressed }) => buttonFeedback(styles.imageModalCloseBtn, pressed)}>
                <Feather color={palette.inkInverse} name="x" size={16} />
                <Text style={styles.imageModalCloseBtnText}>关闭</Text>
              </Pressable>
            </View>
            <Pressable onPress={() => setFullScreen(false)} style={styles.imageModalContent}>
              <Image cachePolicy="memory" contentFit="contain" onError={onError} source={{ uri }} style={styles.imageModalFullImage} />
            </Pressable>
          </View>
        </Modal>
      ) : null}
    </>
  );
}

function fileInitial(name: string) {
  const extension = name.split(".").pop()?.trim();
  return extension && extension !== name ? extension.slice(0, 3).toUpperCase() : "文件";
}
