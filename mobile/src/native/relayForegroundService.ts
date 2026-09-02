import { DeviceEventEmitter, NativeModules, PermissionsAndroid, Platform } from "react-native";

export type RelayForegroundServiceConfig = {
  websocketURL: string;
  userID: string;
  deviceID: string;
  clientToken: string;
};

export type RelayForegroundServiceState = {
  state: "stopped" | "connecting" | "connected" | "reconnecting" | "error" | string;
  status?: string;
  error?: string;
};

type RelayForegroundServiceModule = {
  start: (
    websocketURL: string,
    userID: string,
    deviceID: string,
    clientToken: string,
  ) => Promise<boolean>;
  stop: () => Promise<boolean>;
  send: (message: string) => Promise<boolean>;
  updateStatus: (status: string) => Promise<boolean>;
  isRunning: () => Promise<boolean>;
  getState: () => Promise<RelayForegroundServiceState>;
  drainMessages: () => Promise<string[]>;
};

const nativeModule = NativeModules.RelayForegroundService as RelayForegroundServiceModule | undefined;
export const relayForegroundServiceStateEvent = "RelayForegroundServiceState";
export const relayForegroundServiceMessageEvent = "RelayForegroundServiceMessage";

export function hasRelayForegroundService() {
  return Platform.OS === "android" && Boolean(nativeModule);
}

export function subscribeRelayForegroundService({
  onMessageAvailable,
  onState,
}: {
  onMessageAvailable: () => void;
  onState: (state: RelayForegroundServiceState) => void;
}) {
  if (!hasRelayForegroundService()) {
    return () => undefined;
  }
  const stateSubscription = DeviceEventEmitter.addListener(relayForegroundServiceStateEvent, onState);
  const messageSubscription = DeviceEventEmitter.addListener(relayForegroundServiceMessageEvent, onMessageAvailable);
  return () => {
    stateSubscription.remove();
    messageSubscription.remove();
  };
}

export async function drainRelayForegroundServiceMessages() {
  if (!hasRelayForegroundService() || !nativeModule) {
    return [];
  }
  try {
    return await nativeModule.drainMessages();
  } catch (error) {
    console.warn("Relay foreground service message drain failed", error);
    return [];
  }
}

export async function startRelayForegroundService(config: RelayForegroundServiceConfig) {
  if (Platform.OS !== "android" || !nativeModule) {
    return false;
  }

  // Start while the activity is visible. Permission prompts can suspend the JS
  // continuation, but the foreground-service start itself must not be delayed
  // until after that prompt or a background transition.
  const startPromise = nativeModule.start(
    config.websocketURL,
    config.userID,
    config.deviceID,
    config.clientToken,
  );

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

export async function sendRelayForegroundServiceMessage(message: string) {
  if (Platform.OS !== "android" || !nativeModule) {
    return false;
  }
  try {
    return await nativeModule.send(message);
  } catch (error) {
    console.warn("Relay foreground service send failed", error);
    return false;
  }
}

export async function getRelayForegroundServiceState() {
  if (Platform.OS !== "android" || !nativeModule) {
    return { state: "stopped" } satisfies RelayForegroundServiceState;
  }
  try {
    return await nativeModule.getState();
  } catch (error) {
    console.warn("Relay foreground service state read failed", error);
    return { state: "error", error: String(error) } satisfies RelayForegroundServiceState;
  }
}

export async function isRelayForegroundServiceRunning() {
  if (Platform.OS !== "android" || !nativeModule) {
    return false;
  }
  return nativeModule.isRunning();
}
