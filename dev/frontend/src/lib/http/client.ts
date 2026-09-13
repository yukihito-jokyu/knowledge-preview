import axios, { type AxiosRequestConfig } from "axios";

import { ApiError } from "@/lib/http/api-error";

const configuredApiBaseUrl: unknown = import.meta.env.VITE_API_BASE_URL;
const apiBaseUrl = typeof configuredApiBaseUrl === "string" ? configuredApiBaseUrl : "/api/v1";

export const apiClient = axios.create({
  baseURL: apiBaseUrl,
  allowAbsoluteUrls: false,
  withCredentials: true,
  timeout: 10_000,
  headers: { Accept: "application/json" },
});

apiClient.interceptors.response.use(
  (response) => response,
  (error: unknown) => {
    if (!axios.isAxiosError<unknown>(error)) {
      return Promise.reject(error);
    }

    const body = error.response?.data;

    const envelope =
      typeof body === "object" && body !== null && "error" in body ? body.error : body;

    const data = typeof envelope === "object" && envelope !== null ? envelope : {};

    const message =
      "message" in data && typeof data.message === "string" ? data.message : "API request failed";

    const code = "code" in data && typeof data.code === "string" ? data.code : error.code;

    const details =
      "details" in data && Array.isArray(data.details)
        ? data.details.flatMap((item: unknown) => {
            if (typeof item !== "object" || item === null) return [];
            return [
              {
                field: "field" in item && typeof item.field === "string" ? item.field : undefined,
                reason:
                  "reason" in item && typeof item.reason === "string" ? item.reason : undefined,
                line:
                  "line" in item &&
                  typeof item.line === "number" &&
                  Number.isSafeInteger(item.line) &&
                  item.line > 0
                    ? item.line
                    : undefined,
                column:
                  "column" in item &&
                  typeof item.column === "number" &&
                  Number.isSafeInteger(item.column) &&
                  item.column > 0
                    ? item.column
                    : undefined,
              },
            ];
          })
        : [];

    return Promise.reject(new ApiError(message, error.response?.status ?? 0, code, details));
  },
);

export const httpClient = {
  get: <T>(path: string, config?: AxiosRequestConfig) =>
    apiClient.get<T>(path, config).then((response) => response.data),
  post: <T, D>(path: string, data: D, config?: AxiosRequestConfig<D>) =>
    apiClient.post<T>(path, data, config).then((response) => response.data),
  put: <T, D>(path: string, data: D, config?: AxiosRequestConfig<D>) =>
    apiClient.put<T>(path, data, config).then((response) => response.data),
  patch: <T, D>(path: string, data: D, config?: AxiosRequestConfig<D>) =>
    apiClient.patch<T>(path, data, config).then((response) => response.data),
  delete: <T>(path: string, config?: AxiosRequestConfig) =>
    apiClient.delete<T>(path, config).then((response) => response.data),
};
