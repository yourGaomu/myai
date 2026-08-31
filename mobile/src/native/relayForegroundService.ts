import { PermissionsAndroid, Platform, NativeModules } from "react-native";

type RelayForegroundServiceModule = {
  start: () => Promise<boolean>;
  stop: () => Promise<boolean>;
  updateStatus: (status: string) => Promise<boolean>;
  isRunning: () => Promise<boolean>;
};

const nativeModule = NativeModules.RelayForegroundService as RelayForegroundServiceModule | undefined;

export async function startRelayForegroundService() {
  if (Platform.OS !== "android" || !nativeModule) {
    return false;
  }

  // Start while the activity is visible. Permission prompts can suspend the JS
  // continuation, but the foreground-service start itself must not be delayed
  // until after that prompt or a background transition.
  const startPromise = nativeModule.start();

  if (Platform.Version >= 33 && PermissionsAndroid.check) {
    const permission = "android.permission.POST_NOTIFICATIONS" as const;
    const granted = await PermissionsAndroid.check(permission);
    if (!granted) {
      try {
        await PermissionsAndroid.request(permission, {
          title: "允许后台连接通知",
          message: "MYAI 需要显示常驻通知，才能在后台保持 Relay 连接。",
          buttonPositive: "允许",
          buttonNegative: "暂不允许",
        });
      } catch (error) {
        // Notification permission can only be requested while the activity is visible.
        // The service can still be started; Android will expose it in the task manager
        // even when the notification permission is denied.
        console.warn("Relay notification permission request failed", error);
      }
    }
  }

  await startPromise;
  return true;
}

export async function stopRelayForegroundService() {
  if (Platform.OS !== "android" || !nativeModule) {
    return false;
  }
  await nativeModule.stop();
  return true;
}

export async function updateRelayForegroundServiceStatus(status: string) {
  if (Platform.OS !== "android" || !nativeModule) {
    return false;
  }
  await nativeModule.updateStatus(status);
  return true;
}

export async function isRelayForegroundServiceRunning() {
  if (Platform.OS !== "android" || !nativeModule) {
    return false;
  }
  return nativeModule.isRunning();
}
