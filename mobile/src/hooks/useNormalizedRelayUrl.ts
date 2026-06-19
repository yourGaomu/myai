import { useMemo } from "react";

export function useNormalizedRelayUrl(relayURL: string) {
  return useMemo(() => relayURL.trim().replace(/\/+$/, ""), [relayURL]);
}
