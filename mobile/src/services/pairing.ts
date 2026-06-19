import type { PairResponse } from "../protocol";
import { clientName } from "../utils/relay";

const pairTimeoutMs = 10000;

export async function pairWithRelay(normalizedRelayURL: string, bindCode: string): Promise<PairResponse> {
  const controller = new AbortController();
  const timeoutID = setTimeout(() => controller.abort(), pairTimeoutMs);
  let response: Response;
  try {
    response = await fetch(`${normalizedRelayURL}/pair`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ bind_code: bindCode, client_name: clientName() }),
      signal: controller.signal,
    });
  } catch (error) {
    if (error instanceof Error && error.name === "AbortError") {
      throw new Error("Pair request timed out. Check the relay URL and make sure the relay server is running.");
    }
    throw error;
  } finally {
    clearTimeout(timeoutID);
  }

  if (!response.ok) {
    throw new Error((await response.text()).trim() || response.statusText);
  }

  return (await response.json()) as PairResponse;
}
