import { createFileRoute, Outlet, redirect } from "@tanstack/react-router";
import { AppShell } from "@/components/app-shell";
import { viewerQueryOptions } from "@/features/auth/model/auth-queries";

export const Route = createFileRoute("/_authenticated")({
  beforeLoad: async ({ context, location }) => {
    const viewer = await context.queryClient.query({
      ...viewerQueryOptions(),
      staleTime: "static",
    });

    if (!viewer) {
      throw redirect({
        to: "/login",
        search: {
          redirect: location.href,
        },
      });
    }
  },

  component: () => (
    <AppShell>
      <Outlet />
    </AppShell>
  ),
});
