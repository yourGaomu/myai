import { useCallback, useState } from "react";
import { SafeAreaProvider } from "react-native-safe-area-context";

import { StartupLoadingOverlay } from "./src/components/startup/StartupLoadingOverlay";
import { OtaUpdateManager } from "./src/components/updates/OtaUpdateManager";
import { MobileAppScreen } from "./src/screens";

export default function App() {
  const [startupVisible, setStartupVisible] = useState(true);
  const finishStartup = useCallback(() => setStartupVisible(false), []);

  return (
    <SafeAreaProvider>
      <MobileAppScreen />
      <StartupLoadingOverlay onDone={finishStartup} visible={startupVisible} />
      <OtaUpdateManager />
    </SafeAreaProvider>
  );
}
