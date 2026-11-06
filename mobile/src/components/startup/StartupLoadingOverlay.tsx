import { useEffect, useRef, useState } from "react";
import { Animated, Image, StyleSheet, Text, View } from "react-native";
import { LinearGradient } from "expo-linear-gradient";
import { font, palette, radius, weight } from "../../constants/theme";

const workingFrames = [
  require("../../../assets/loading/character-01.png"),
  require("../../../assets/loading/character-02.png"),
  require("../../../assets/loading/character-03.png"),
  require("../../../assets/loading/character-04.png"),
  require("../../../assets/loading/character-05.png"),
  require("../../../assets/loading/character-06.png"),
  require("../../../assets/loading/character-07.png"),
  require("../../../assets/loading/character-08.png"),
  require("../../../assets/loading/character-09.png"),
  require("../../../assets/loading/character-10.png"),
];

const thinkingFrames = [
  require("../../../assets/loading/thinking/character-01.png"),
  require("../../../assets/loading/thinking/character-02.png"),
  require("../../../assets/loading/thinking/character-03.png"),
  require("../../../assets/loading/thinking/character-04.png"),
  require("../../../assets/loading/thinking/character-05.png"),
  require("../../../assets/loading/thinking/character-06.png"),
  require("../../../assets/loading/thinking/character-07.png"),
  require("../../../assets/loading/thinking/character-08.png"),
  require("../../../assets/loading/thinking/character-09.png"),
  require("../../../assets/loading/thinking/character-10.png"),
];

const loadingFrames = [
  ...workingFrames.map((source) => ({ source, status: "正在启动..." })),
  ...thinkingFrames.map((source) => ({ source, status: "正在思考..." })),
];

type Props = {
  onDone: () => void;
  visible: boolean;
};

export function StartupLoadingOverlay({ onDone, visible }: Props) {
  const [frame, setFrame] = useState(0);
  const opacity = useRef(new Animated.Value(0)).current;
  const translateY = useRef(new Animated.Value(10)).current;

  useEffect(() => {
    if (!visible) {
      return;
    }

    setFrame(0);
    opacity.setValue(0);
    translateY.setValue(10);

    const frameTimer = setInterval(() => {
      setFrame((current) => (current + 1) % loadingFrames.length);
    }, 100);
    const finishTimer = setTimeout(() => {
      Animated.parallel([
        Animated.timing(opacity, { duration: 220, toValue: 0, useNativeDriver: true }),
        Animated.timing(translateY, { duration: 220, toValue: -8, useNativeDriver: true }),
      ]).start(({ finished }) => {
        if (finished) {
          onDone();
        }
      });
    }, 2200);

    Animated.parallel([
      Animated.timing(opacity, { duration: 260, toValue: 1, useNativeDriver: true }),
      Animated.spring(translateY, { bounciness: 5, speed: 16, toValue: 0, useNativeDriver: true }),
    ]).start();

    return () => {
      clearInterval(frameTimer);
      clearTimeout(finishTimer);
      opacity.stopAnimation();
      translateY.stopAnimation();
    };
  }, [onDone, opacity, translateY, visible]);

  if (!visible) {
    return null;
  }

  return (
    <View pointerEvents="auto" style={styles.overlay}>
      <Animated.View style={[styles.content, { opacity, transform: [{ translateY }] }]}>
        <Image
          accessibilityLabel="MYAI 正在启动"
          source={loadingFrames[frame].source}
          style={styles.character}
        />
        <Text style={styles.title}>MYAI</Text>
        <Text style={styles.status}>{loadingFrames[frame].status}</Text>
        <View style={styles.progressTrack}>
          <LinearGradient
            colors={[palette.brandLight, palette.brand]}
            end={{ x: 1, y: 0 }}
            start={{ x: 0, y: 0 }}
            style={[styles.progressFill, { width: `${((frame + 1) / loadingFrames.length) * 100}%` }]}
          />
        </View>
      </Animated.View>
    </View>
  );
}

const styles = StyleSheet.create({
  overlay: {
    alignItems: "center",
    backgroundColor: palette.canvas,
    bottom: 0,
    justifyContent: "center",
    left: 0,
    position: "absolute",
    right: 0,
    top: 0,
    zIndex: 100,
  },
  content: {
    alignItems: "center",
    justifyContent: "center",
    width: "100%",
  },
  character: {
    height: 360,
    width: 144,
  },
  title: {
    color: palette.ink1,
    fontSize: font.display,
    fontWeight: weight.heavy,
    marginTop: -8,
  },
  status: {
    color: palette.ink2,
    fontSize: font.label,
    fontWeight: weight.medium,
    marginTop: 4,
  },
  progressTrack: {
    backgroundColor: palette.surface2,
    borderColor: palette.hairline,
    borderRadius: radius.pill,
    borderWidth: 1,
    height: 6,
    marginTop: 14,
    overflow: "hidden",
    width: 150,
  },
  progressFill: {
    height: "100%",
  },
});
