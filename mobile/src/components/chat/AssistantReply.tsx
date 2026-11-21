import { useMemo } from "react";
import { View } from "react-native";

import type { AssetSummary } from "../../protocol";
import type { ButtonFeedback } from "../../types/ui";
import { assistantReplyAssets } from "../../utils/chatMarkdown";
import type { SharedAsset } from "../../utils/toolAssets";
import { MarkdownText } from "./MarkdownText";
import { SharedAssetCard } from "./SharedAssetCard";
import { useChatStyles } from "./styles";

export function AssistantReply({ assets, buttonFeedback, catalog, sessionID, text }: {
  assets?: SharedAsset[];
  buttonFeedback: ButtonFeedback;
  catalog?: AssetSummary[];
  sessionID?: string;
  text: string;
}) {
  const styles = useChatStyles();
  const cards = useMemo(() => assistantReplyAssets(text, assets, catalog), [assets, catalog, text]);
  return (
    <>
      {text ? <MarkdownText text={text} /> : null}
      {cards.length > 0 ? (
        <View style={styles.messageAttachments}>
          {cards.map(({ asset, imageReference }) => (
            <SharedAssetCard asset={asset} buttonFeedback={buttonFeedback} imageReference={imageReference} key={asset.assetID} sessionID={sessionID} />
          ))}
        </View>
      ) : null}
    </>
  );
}
