import { useEffect, useState } from "react";
import AsyncStorage from "@react-native-async-storage/async-storage";

import { defaultRelayURL, mobileSettingsKey } from "../constants/app";

type MobileSettings = {
  clientToken: string;
  deviceID: string;
  relayURL: string;
  userID: string;
};

type UseMobileSettingsOptions = {
  onTokenRestored?: () => void;
};

export function useMobileSettings({ onTokenRestored }: UseMobileSettingsOptions = {}) {
  const [relayURL, setRelayURL] = useState(defaultRelayURL);
  const [userID, setUserID] = useState("local");
  const [deviceID, setDeviceID] = useState("pc-local");
  const [clientToken, setClientToken] = useState("");
  const [settingsLoaded, setSettingsLoaded] = useState(false);

  useEffect(() => {
    let mounted = true;
    AsyncStorage.getItem(mobileSettingsKey)
      .then((raw) => {
        if (!raw || !mounted) {
          return;
        }
        const settings = JSON.parse(raw) as Partial<MobileSettings>;
        if (settings.relayURL) {
          setRelayURL(settings.relayURL);
        }
        if (settings.userID) {
          setUserID(settings.userID);
        }
        if (settings.deviceID) {
          setDeviceID(settings.deviceID);
        }
        if (settings.clientToken) {
          setClientToken(settings.clientToken);
          onTokenRestored?.();
        }
      })
      .catch(() => undefined)
      .finally(() => {
        if (mounted) {
          setSettingsLoaded(true);
        }
      });
    return () => {
      mounted = false;
    };
  }, [onTokenRestored]);

  useEffect(() => {
    if (!settingsLoaded) {
      return;
    }
    const settings: MobileSettings = {
      relayURL,
      userID,
      deviceID,
      clientToken,
    };
    AsyncStorage.setItem(mobileSettingsKey, JSON.stringify(settings)).catch(() => undefined);
  }, [clientToken, deviceID, relayURL, settingsLoaded, userID]);

  return {
    clientToken,
    deviceID,
    relayURL,
    setClientToken,
    setDeviceID,
    setRelayURL,
    setUserID,
    settingsLoaded,
    userID,
  };
}
