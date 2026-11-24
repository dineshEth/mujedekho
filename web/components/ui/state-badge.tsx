import type { BadgeTone } from "./badge";
import { Badge } from "./badge";
import type { MonitorState, CheckStatus } from "@/lib/types";

export function StateBadge({ state }: { state: MonitorState | CheckStatus }) {
  const tone: BadgeTone = state === "up" ? "up" : state === "down" || state === "timeout" ? "down" : "paused";
  const label = state === "timeout" ? "timeout" : state;
  return <Badge tone={tone}>{label}</Badge>;
}
