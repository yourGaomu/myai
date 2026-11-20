import AsyncStorage from "@react-native-async-storage/async-storage";
import { useCallback, useEffect, useRef, useState } from "react";

import { defaultRelayURL, mobileSettingsKey } from "../constants/app";
import type { RelayScope } from "../types/app";
import { restoreRelaySettings, type RelayPairing } from "../utils/relayPairing";

// Pairing and issuer are stored together. The editable address never changes an
// existing connection or its saved settings until a new pairing succeeds.
export function useMobileSettings() {
  const [relayURL, setRelayURL] = useState(defaultRelayURL);
  const [pairing, setPairing] = useState<RelayPairing | null>(null);
  const [settingsLoaded, setSettingsLoaded] = useState(false);
  const currentPairingRef = useRef<RelayPairing | null>(null);
  const relayScopeRef = useRef<RelayScope>({ pairingID: "", connectionID: "" });
  const relayURLRef = useRef(defaultRelayURL);
  const persistedWrites = useRef(Promise.resolve());

  const persist = useCallback((settings: RelayPairing | { relayURL: string }) => {
    persistedWrites.current = persistedWrites.current
      .then(() => AsyncStorage.setItem(mobileSettingsKey, JSON.stringify(settings)))
      .catch(() => undefined);
  }, []);

  const editRelayURL = useCallback((url: string) => {
    relayURLRef.current = url;
    setRelayURL(url);
  }, []);

  useEffect(() => {
    let mounted = true;
    void AsyncStorage.getItem(mobileSettingsKey)
      .then((raw) => {
        if (!mounted || !raw) return;
        const restored = restoreRelaySettings(JSON.parse(raw));
        editRelayURL(restored.relayURL || defaultRelayURL);
        currentPairingRef.current = restored.pairing;
        relayScopeRef.current = { pairingID: restored.pairing?.pairingID || "", connectionID: "" };
        setPairing(restored.pairing);
        if (restored.pairing) persist(restored.pairing);
      })
      .catch(() => undefined)
      .finally(() => { if (mounted) setSettingsLoaded(true); });
    return () => { mounted = false; };
  }, [editRelayURL, persist]);

  const commitPairing = useCallback((next: RelayPairing) => {
    currentPairingRef.current = next;
    relayScopeRef.current = { pairingID: next.pairingID, connectionID: "" };
    setPairing(next);
    editRelayURL(next.relayURL);
    persist(next);
  }, [editRelayURL, persist]);

  return { commitPairing, currentPairingRef, editRelayURL, pairing, relayURL, relayURLRef, relayScopeRef, settingsLoaded };
}
