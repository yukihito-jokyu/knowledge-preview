import { queryOptions } from "@tanstack/react-query";
import { authApi } from "@/features/auth/api/auth-api";
import { authQueryKeys } from "@/features/auth/model/auth-query-keys";

export const viewerQueryOptions = () =>
  queryOptions({ queryKey: authQueryKeys.viewer, queryFn: authApi.getViewer });
