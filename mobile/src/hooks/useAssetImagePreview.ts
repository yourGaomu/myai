import { createContext, useCallback, useLayoutEffect, useMemo, useRef, useState, type RefObject } from "react";

import type { RelayMessage } from "../protocol";
import type { RelayScope } from "../types/app";
import { createAssetImagePreviewLoader, type AssetImagePreviewLoader } from "../utils/assetImagePreview";

export const AssetPreviewConnectionContext = createContext<{ connectionID: string; loader: AssetImagePreviewLoader } | null>(null);
// A long chat retains its ScrollView rows. Only rows near the viewport may
// request previews and hold their decoded source.
export const AssetPreviewVisibleContext = createContext(true);

export function useAssetImagePreview({
  relayScopeRef, sessionIDRef, sessionID, inspectionSessionID, sendEnvelope,
}: {
  relayScopeRef: RefObject<RelayScope>;
  sessionIDRef: RefObject<string>;
  sessionID: string;
  inspectionSessionID?: string;
  sendEnvelope: (type: RelayMessage["type"], overrides?: Partial<RelayMessage>) => boolean;
}) {
  const inspectorRef = useRef<{ parentID: string; childID?: string } | null>(null);
  const loader = useMemo(() => createAssetImagePreviewLoader({
    getScope: () => relayScopeRef.current,
    isSessionCurrent: (target) => target === sessionIDRef.current || (
      inspectorRef.current?.parentID === sessionIDRef.current && target === inspectorRef.current.childID
    ),
    sendEnvelope,
  }), [relayScopeRef, sendEnvelope, sessionIDRef]);
  const [connectionID, setConnectionID] = useState("");
  useLayoutEffect(() => {
    inspectorRef.current = { parentID: sessionID, childID: inspectionSessionID };
    loader.prune();
  }, [inspectionSessionID, loader, sessionID]);
  useLayoutEffect(() => () => loader.clear(), [loader]);

  const syncPreviewConnection = useCallback(() => {
    loader.prune();
    setConnectionID(relayScopeRef.current.connectionID);
  }, [loader, relayScopeRef]);
  const previewConnection = useMemo(() => ({ connectionID, loader }), [connectionID, loader]);
  return { previewConnection, syncPreviewConnection, handleAssetPreviewMessage: loader.handleMessage };
}
