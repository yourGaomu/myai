import * as Clipboard from "expo-clipboard";
import { Platform } from "react-native";

export async function copyToClipboard(text: string): Promise<boolean> {
  const value = text ?? "";
  if (!value) return false;
  try {
    await Clipboard.setStringAsync(value);
    return true;
  } catch {
    if (Platform.OS === "web" && typeof navigator !== "undefined" && navigator.clipboard?.writeText) {
      try {
        await navigator.clipboard.writeText(value);
        return true;
      } catch {
        return false;
      }
    }
    return false;
  }
}
