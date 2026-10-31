import { redirect } from "next/navigation";

/**
 * The terminal opens on the market, so the group root has no page of its own.
 */
export default function TerminalIndex() {
  redirect("/market");
}
