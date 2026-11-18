import { useEffect, useRef, useState } from "react";
import { AppState, StyleSheet, View, type StyleProp, type ViewStyle } from "react-native";
import { BlurTargetView, BlurView } from "expo-blur";
import { Image as ExpoImage } from "expo-image";
import { LinearGradient } from "expo-linear-gradient";
import { useVideoPlayer, VideoView } from "expo-video";

import { useWallpaper } from "../../hooks/useWallpaperConfig";
import { useAppTheme } from "../../theme/ThemeProvider";
import {
  getWallpaperPreset,
  type WallpaperResizeMode,
  type WallpaperSettings,
} from "../../utils/wallpaperRegistry";

type Props = {
  /** 仅当处于前台且可见聊天页时为 true；关闭时自动暂停视频节省电量 */
  active?: boolean;
  /** 允许预览面板传入临时配置预览 */
  settingsOverride?: WallpaperSettings;
  style?: StyleProp<ViewStyle>;
};

export function ChatWallpaperLayer({ active = true, settingsOverride, style }: Props) {
  const { settings: contextSettings } = useWallpaper();
  const { isDark, palette } = useAppTheme();
  const settings = settingsOverride ?? contextSettings;

  const [appActive, setAppActive] = useState(() => AppState.currentState === "active");
  const blurTargetRef = useRef<View | null>(null);

  useEffect(() => {
    const sub = AppState.addEventListener("change", (state) => {
      setAppActive(state === "active");
    });
    return () => sub.remove();
  }, []);

  const enabled =
    settings.kind === "preset" || ((settings.kind === "image" || settings.kind === "video") && Boolean(settings.localUri));
  if (!enabled) {
    return null;
  }

  const preset = getWallpaperPreset(settings.presetId);
  const playing = active && appActive;
  const overlayColor = isDark
    ? `rgba(10, 12, 16, ${settings.dimOpacity})`
    : `rgba(246, 248, 252, ${settings.dimOpacity})`;
  const topVignette = isDark ? "rgba(10, 12, 16, 0.36)" : "rgba(245, 247, 250, 0.32)";
  const bottomVignette = isDark ? "rgba(10, 12, 16, 0.44)" : "rgba(245, 247, 250, 0.38)";

  return (
    <View pointerEvents="none" style={[StyleSheet.absoluteFill, { backgroundColor: palette.canvas }, style]}>
      <BlurTargetView ref={blurTargetRef} style={StyleSheet.absoluteFill}>
        {settings.kind === "preset" ? (
          <LinearGradient
            colors={isDark ? preset.darkColors : preset.lightColors}
            end={{ x: 0.9, y: 1 }}
            locations={preset.locations}
            start={{ x: 0.1, y: 0 }}
            style={StyleSheet.absoluteFill}
          />
        ) : null}

        {settings.kind === "image" && settings.localUri ? (
          <ExpoImage
            autoplay
            contentFit={settings.resizeMode}
            source={{ uri: settings.localUri }}
            style={StyleSheet.absoluteFill}
            transition={180}
          />
        ) : null}

        {settings.kind === "video" && settings.localUri ? (
          <WallpaperVideoPlayer
            key={settings.localUri}
            playing={playing}
            resizeMode={settings.resizeMode}
            uri={settings.localUri}
          />
        ) : null}
      </BlurTargetView>

      {settings.blurAmount > 0 ? (
        <BlurView
          blurMethod="dimezisBlurViewSdk31Plus"
          blurTarget={blurTargetRef}
          intensity={settings.blurAmount}
          pointerEvents="none"
          style={StyleSheet.absoluteFill}
          tint={isDark ? "dark" : "light"}
        />
      ) : null}

      <View pointerEvents="none" style={[StyleSheet.absoluteFill, { backgroundColor: overlayColor }]} />

      <LinearGradient
        colors={[topVignette, "transparent", "transparent", bottomVignette]}
        locations={[0, 0.22, 0.78, 1]}
        pointerEvents="none"
        style={StyleSheet.absoluteFill}
      />
    </View>
  );
}

function WallpaperVideoPlayer({
  playing,
  resizeMode,
  uri,
}: {
  playing: boolean;
  resizeMode: WallpaperResizeMode;
  uri: string;
}) {
  const [ready, setReady] = useState(false);

  const player = useVideoPlayer(uri, (instance) => {
    instance.loop = true;
    instance.muted = true;
    instance.audioMixingMode = "mixWithOthers";
    instance.keepScreenOnWhilePlaying = false;
    instance.staysActiveInBackground = false;
    if (playing) {
      instance.play();
    }
  });

  useEffect(() => {
    try {
      if (playing) {
        player.play();
      } else {
        player.pause();
      }
    } catch {
      // ignore if player is releasing on fast unmount
    }
  }, [player, playing]);

  return (
    <VideoView
      allowsPictureInPicture={false}
      contentFit={resizeMode}
      nativeControls={false}
      onFirstFrameRender={() => setReady(true)}
      player={player}
      playsInline
      pointerEvents="none"
      style={[StyleSheet.absoluteFill, !ready && styles.hiddenUntilFirstFrame]}
      surfaceType="textureView"
      useExoShutter={false}
    />
  );
}

const styles = StyleSheet.create({
  hiddenUntilFirstFrame: {
    opacity: 0,
  },
});
