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

    const responseMessage = error.response?.data;

    const message =
      typeof responseMessage === "object" &&
      responseMessage !== null &&
      "message" in responseMessage &&
      typeof responseMessage.message === "string"
        ? responseMessage.message
        : error.message;

    return Promise.reject(new ApiError(message, error.response?.status ?? 0, error.code));
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
