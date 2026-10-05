import createClient from "openapi-fetch";
import type { paths } from "./schema";

// Delegate to globalThis.fetch so tests can stub networking.
const fetchImpl: typeof globalThis.fetch = (input, init) => globalThis.fetch(input, init);

// Absolute base URL: relative URLs do not resolve under Node/jsdom (tests), and in the
// browser this keeps calls same-origin so the dev proxy (vite.config.ts) can route them.
const baseUrl = globalThis.location?.origin ?? "http://localhost:8080";

export const apiClient = createClient<paths>({ baseUrl, fetch: fetchImpl });

export type HealthResponse =
  paths["/healthz"]["get"]["responses"]["200"]["content"]["application/json"];

/** Fetch the API health status through the generated OpenAPI client. */
export async function fetchHealth(): Promise<HealthResponse> {
  const { data, response } = await apiClient.GET("/healthz");
  if (data === undefined) {
    throw new Error(`health check failed with status ${response.status}`);
  }
  return data;
}
