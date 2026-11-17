import { useCallback, useState } from "react";
import { SafeAreaProvider } from "react-native-safe-area-context";

import { StartupLoadingOverlay } from "./src/components/startup/StartupLoadingOverlay";
import { OtaUpdateManager } from "./src/components/updates/OtaUpdateManager";
import { MotionProvider } from "./src/hooks/useMotionConfig";
import { MobileAppScreen } from "./src/screens";
import { ThemeProvider } from "./src/theme/ThemeProvider";

export default function App() {
  const [startupVisible, setStartupVisible] = useState(true);
  const finishStartup = useCallback(() => setStartupVisible(false), []);

  return (
    <SafeAreaProvider>
      <ThemeProvider>
        <MotionProvider>
          <MobileAppScreen />
          <StartupLoadingOverlay onDone={finishStartup} visible={startupVisible} />
          <OtaUpdateManager />
        </MotionProvider>
      </ThemeProvider>
    </SafeAreaProvider>
  );
}
