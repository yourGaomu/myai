import { clientName } from "../utils/relay";
import { normalizeRelayUrl, readPairingResponse } from "../utils/relayPairing";

const pairTimeoutMs = 10000;

export async function pairWithRelay(relayURL: string, bindCode: string, signal: AbortSignal) {
  const controller = new AbortController();
  const cancel = () => controller.abort();
  signal.addEventListener("abort", cancel, { once: true });
  if (signal.aborted) cancel();
  const timeoutID = setTimeout(cancel, pairTimeoutMs);
  try {
    const response = await fetch(`${relayURL}/pair`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ bind_code: bindCode, client_name: clientName() }),
      signal: controller.signal,
      redirect: "error",
    });
    // Some native fetch implementations follow redirects despite the option.
    if (response.url && normalizeRelayUrl(response.url) !== `${relayURL}/pair`) {
      throw new Error("配对请求被重定向，请使用中继的实际配对地址。");
    }
    if (!response.ok) {
      if (response.status === 429) throw new Error("配对尝试过多，请稍后重试。");
      if (response.status === 404) throw new Error("配对码无效、已使用或已过期。");
      throw new Error(`配对请求失败（HTTP ${response.status}）。`);
    }
    return readPairingResponse(await response.json());
  } catch (error) {
    if (controller.signal.aborted && !signal.aborted) throw new Error("配对超时，请检查中继地址和网络。");
    if (error instanceof TypeError) throw new Error("无法连接配对服务，请检查中继地址和网络。");
    throw error;
  } finally {
    signal.removeEventListener("abort", cancel);
    clearTimeout(timeoutID);
  }
}
