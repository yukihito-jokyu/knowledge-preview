import { httpClient } from "@/lib/http/client";
import type { Viewer } from "@/features/auth/model/auth-types";

export const authApi = { getViewer: () => httpClient.get<Viewer | null>("/auth/me") };
