import { useEffect, useRef, useState, type RefObject } from "react";

import { pairWithRelay } from "../services/pairing";
import { messageFromError } from "../utils/format";
import { newRequestID } from "../utils/ids";
import { normalizeRelayUrl, type RelayPairing } from "../utils/relayPairing";

type Args = {
  relayURLRef: RefObject<string>;
  onPaired: (pairing: RelayPairing) => void;
};

export function usePairingActions({ relayURLRef, onPaired }: Args) {
  const [bindCode, setBindCode] = useState("");
  const [pairingPending, setPairingPending] = useState(false);
  const [pairingError, setPairingError] = useState("");
  const pendingRef = useRef<AbortController | null>(null);

  useEffect(() => () => {
    pendingRef.current?.abort();
    pendingRef.current = null;
  }, []);

  function cancelPairing() {
    pendingRef.current?.abort();
    pendingRef.current = null;
    setPairingPending(false);
    setPairingError("");
  }

  async function pairDevice() {
    if (pendingRef.current) return;
    const code = bindCode.trim();
    const relayURL = normalizeRelayUrl(relayURLRef.current);
    if (!code || !relayURL) {
      setPairingError("请输入有效的中继地址和配对码。");
      return;
    }

    const attempt = new AbortController();
    pendingRef.current = attempt;
    setPairingPending(true);
    setPairingError("");
    try {
      const result = await pairWithRelay(relayURL, code, attempt.signal);
      if (pendingRef.current === attempt && normalizeRelayUrl(relayURLRef.current) === relayURL) {
        onPaired({ ...result, pairingID: newRequestID(), relayURL });
        setBindCode("");
      }
    } catch (error) {
      if (pendingRef.current === attempt) setPairingError(messageFromError(error));
    } finally {
      if (pendingRef.current === attempt) {
        pendingRef.current = null;
        setPairingPending(false);
      }
    }
  }

  return { bindCode, cancelPairing, pairDevice, pairingError, pairingPending, setBindCode };
}
