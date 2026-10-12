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

const prototypeRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/prototype",
  // Lazy: the same-clock causal prototype viewer only loads when visited.
  component: lazyRouteComponent(() => import("./pages/PrototypePage"), "PrototypePage"),
});

export const router = createRouter({
  routeTree: rootRoute.addChildren([homeRoute, viewerRoute, prototypeRoute]),
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
