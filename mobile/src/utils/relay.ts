import { Platform } from "react-native";

export { websocketURL } from "./relayPairing";

export function clientName() {
  return `Mobile ${Platform.OS}`;
}
