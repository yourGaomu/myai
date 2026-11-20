import { DeviceEventEmitter, NativeModules, PermissionsAndroid, Platform } from "react-native";

import { websocketURL, type RelayPairing } from "../utils/relayPairing";

export type RelayForegroundServiceState = {
  state: "stopped" | "connecting" | "authenticating" | "connected" | "reconnecting" | "auth_rejected" | "error";
  status?: string;
  error?: string;
  pairingID: string;
  connectionID: string;
  revision: number;
};

type RelayForegroundServiceModule = {
  startScoped: (
    websocketURL: string, userID: string, deviceID: string, clientToken: string, pairingID: string, retry: boolean,
  ) => Promise<boolean>;
  stopScoped: (pairingID: string) => Promise<boolean>;
  sendScoped: (pairingID: string, connectionID: string, message: string) => Promise<boolean>;
  getScopedState: () => Promise<RelayForegroundServiceState>;
  drainScopedMessages: (pairingID: string, connectionID: string) => Promise<string[]>;
  stop?: () => Promise<boolean>;
};

const nativeModule = NativeModules.RelayForegroundService as Partial<RelayForegroundServiceModule> | undefined;
let askedForNotification = false;

export function hasRelayForegroundService(): boolean {
  return Platform.OS === "android" && typeof nativeModule?.startScoped === "function" &&
    typeof nativeModule.stopScoped === "function" && typeof nativeModule.sendScoped === "function" &&
    typeof nativeModule.getScopedState === "function" && typeof nativeModule.drainScopedMessages === "function";
}

export function subscribeRelayForegroundService({
  onMessageAvailable,
  onState,
}: {
  onMessageAvailable: () => void;
  onState: (state: RelayForegroundServiceState) => void;
}) {
  const states = DeviceEventEmitter.addListener("RelayForegroundServiceState", onState);
  const messages = DeviceEventEmitter.addListener("RelayForegroundServiceMessage", onMessageAvailable);
  return () => {
    states.remove();
    messages.remove();
  };
}

export function drainRelayForegroundServiceMessages(pairingID: string, connectionID: string) {
  return nativeModule?.drainScopedMessages?.(pairingID, connectionID) ?? Promise.resolve([]);
}

export async function startRelayForegroundService(pairing: RelayPairing, retry: boolean) {
  if (!nativeModule?.startScoped) return false;
  const started = await nativeModule.startScoped(
    websocketURL(pairing.relayURL), pairing.userID, pairing.deviceID, pairing.clientToken, pairing.pairingID, retry,
  );
  if (started && !askedForNotification && Number(Platform.Version) >= 33) {
    askedForNotification = true;
    void PermissionsAndroid.request(PermissionsAndroid.PERMISSIONS.POST_NOTIFICATIONS).catch(() => undefined);
  }
  return started;
}

export function stopRelayForegroundService(pairingID: string) {
  return nativeModule?.stopScoped?.(pairingID) ?? Promise.resolve(false);
}

export function stopLegacyRelayForegroundService() {
  if (Platform.OS === "android" && !hasRelayForegroundService()) {
    return nativeModule?.stop?.() ?? Promise.resolve(false);
  }
  return Promise.resolve(false);
}

export function sendRelayForegroundServiceMessage(pairingID: string, connectionID: string, message: string) {
  return nativeModule?.sendScoped?.(pairingID, connectionID, message) ?? Promise.resolve(false);
}

export function getRelayForegroundServiceState() {
  return nativeModule?.getScopedState?.() ?? Promise.resolve(null);
}
