import { SafeAreaProvider } from "react-native-safe-area-context";

import { MobileAppScreen } from "./src/screens";

export default function App() {
  return (
    <SafeAreaProvider>
      <MobileAppScreen />
    </SafeAreaProvider>
  );
}
