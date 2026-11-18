import { useCallback, useState } from "react";
import { SafeAreaProvider } from "react-native-safe-area-context";

import { StartupLoadingOverlay } from "./src/components/startup/StartupLoadingOverlay";
import { OtaUpdateManager } from "./src/components/updates/OtaUpdateManager";
import { MotionProvider } from "./src/hooks/useMotionConfig";
import { WallpaperProvider } from "./src/hooks/useWallpaperConfig";
import { MobileAppScreen } from "./src/screens";
import { ThemeProvider } from "./src/theme/ThemeProvider";

export default function App() {
  const [startupVisible, setStartupVisible] = useState(true);
  const finishStartup = useCallback(() => setStartupVisible(false), []);

  return (
    <SafeAreaProvider>
      <ThemeProvider>
        <WallpaperProvider>
          <MotionProvider>
            <MobileAppScreen />
            <StartupLoadingOverlay onDone={finishStartup} visible={startupVisible} />
            <OtaUpdateManager />
          </MotionProvider>
        </WallpaperProvider>
      </ThemeProvider>
    </SafeAreaProvider>
  );
}
