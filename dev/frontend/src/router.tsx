import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createRouter } from "@tanstack/react-router";

import { routeTree } from "@/routeTree.gen";
import type { RouterContext } from "@/router-context";

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
    },
  },
});

export const router = createRouter({
  routeTree,
  context: { queryClient } satisfies RouterContext,
  Wrap: ({ children }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  ),
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
