import { Platform } from "react-native";

export function websocketURL(relayURL: string) {
  if (relayURL.startsWith("https://")) {
    return relayURL.replace(/^https:\/\//, "wss://") + "/ws/client";
  }
  if (relayURL.startsWith("http://")) {
    return relayURL.replace(/^http:\/\//, "ws://") + "/ws/client";
  }
  return `ws://${relayURL}/ws/client`;
}

export function clientName() {
  return `Mobile ${Platform.OS}`;
}
