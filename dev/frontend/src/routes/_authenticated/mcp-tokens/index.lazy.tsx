import { createLazyFileRoute } from "@tanstack/react-router";
import { McpTokenPage } from "@/features/mcp-tokens/ui/pages/mcp-token-page";

export const Route = createLazyFileRoute("/_authenticated/mcp-tokens/")({
  component: McpTokenPage,
});
