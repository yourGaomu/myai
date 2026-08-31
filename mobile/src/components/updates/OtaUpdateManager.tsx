import * as Updates from "expo-updates";
import { useCallback, useEffect, useRef } from "react";
import { Alert, AppState, Platform, type AppStateStatus } from "react-native";

const INITIAL_CHECK_DELAY_MS = 2000;
const FOREGROUND_CHECK_COOLDOWN_MS = 5 * 60 * 1000;

export function OtaUpdateManager() {
  const checkingRef = useRef(false);
  const downloadedRef = useRef(false);
  const lastCheckAtRef = useRef(0);

  const checkForUpdate = useCallback(async (force = false) => {
    if (Platform.OS === "web" || !Updates.isEnabled || checkingRef.current || downloadedRef.current) {
      return;
    }

    const now = Date.now();
    if (!force && now - lastCheckAtRef.current < FOREGROUND_CHECK_COOLDOWN_MS) {
      return;
    }
    lastCheckAtRef.current = now;
    checkingRef.current = true;

    try {
      const update = await Updates.checkForUpdateAsync();
      if (!update.isAvailable) {
        return;
      }

      const fetched = await Updates.fetchUpdateAsync();
      if (!fetched.isNew) {
        return;
      }

      downloadedRef.current = true;
      Alert.alert(
        "发现新版本",
        "更新已下载，立即重启应用即可使用最新版本。",
        [
          { text: "稍后", style: "cancel" },
          {
            text: "立即重启",
            onPress: () => {
              void Updates.reloadAsync();
            },
          },
        ],
      );
    } catch (error) {
      // OTA failure must never prevent the embedded bundle from starting.
      console.warn("OTA update check failed", error);
    } finally {
      checkingRef.current = false;
    }
  }, []);

  useEffect(() => {
    const timer = setTimeout(() => {
      void checkForUpdate(true);
    }, INITIAL_CHECK_DELAY_MS);
    return () => clearTimeout(timer);
  }, [checkForUpdate]);

  useEffect(() => {
    let previousState: AppStateStatus = AppState.currentState;
    const subscription = AppState.addEventListener("change", (nextState) => {
      const returnedToForeground =
        previousState.match(/inactive|background/) && nextState === "active";
      previousState = nextState;
      if (returnedToForeground) {
        void checkForUpdate();
      }
    });
    return () => subscription.remove();
  }, [checkForUpdate]);

  return null;
}
