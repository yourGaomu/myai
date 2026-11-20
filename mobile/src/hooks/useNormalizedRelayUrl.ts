import { useMemo } from "react";

import { normalizeRelayUrl } from "../utils/relayPairing";

export function useNormalizedRelayUrl(relayURL: string) {
  return useMemo(() => normalizeRelayUrl(relayURL), [relayURL]);
}
