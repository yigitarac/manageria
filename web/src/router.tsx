import {
  createRootRoute,
  createRoute,
  createRouter,
  lazyRouteComponent,
  Outlet,
} from "@tanstack/react-router";
import HomePage from "./pages/HomePage";

const rootRoute = createRootRoute({ component: () => <Outlet /> });

const homeRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: HomePage,
});

const viewerRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/viewer",
  // Lazy: the PixiJS viewer bundle only loads when the route is visited.
  component: lazyRouteComponent(() => import("./pages/ViewerPage"), "ViewerPage"),
});

export const router = createRouter({
  routeTree: rootRoute.addChildren([homeRoute, viewerRoute]),
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
