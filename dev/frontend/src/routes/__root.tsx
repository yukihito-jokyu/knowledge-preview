import { Outlet, createRootRouteWithContext } from "@tanstack/react-router";
import type { RouterContext } from "@/router-context";

const createRootRoute = createRootRouteWithContext<RouterContext>();

export const Route = createRootRoute({
  component: Outlet,
});
