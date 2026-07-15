import { EvelentError } from "./errors";
import axios, { AxiosResponse, AxiosError } from "axios";

export type HttpClientOptions = {
  apiKey?: string;
};

export class HttpClient {
  private readonly apiKey?: string;

  constructor(
    private readonly baseUrl: string,
    options?: HttpClientOptions,
  ) {
    this.apiKey = options?.apiKey;
  }

  normalizeBase(): string {
    return this.baseUrl.replace(/\/+$/, "");
  }

  /** Headers merged into every request (auth + Accept for SSE callers). */
  authHeaders(extra?: Record<string, string>): Record<string, string> {
    const h: Record<string, string> = { ...(extra || {}) };
    if (this.apiKey) {
      h["X-API-Key"] = this.apiKey;
    }
    return h;
  }

  async request<T>(path: string, init?: RequestInit): Promise<T> {
    const url = `${this.normalizeBase()}${path}`;

    try {
      const response: AxiosResponse = await axios({
        url,
        method: init?.method || "GET",
        headers: {
          "Content-Type": "application/json",
          ...this.authHeaders(init?.headers as Record<string, string>),
        },
        data: init?.body,
        validateStatus: () => true,
      });

      if (response.status < 200 || response.status >= 300) {
        throw new EvelentError(
          response.status,
          response.statusText,
          response.data,
        );
      }

      if (response.status === 204) {
        return {} as T;
      }

      if (response.data == null || response.data === "") {
        return {} as T;
      }

      if (typeof response.data === "string") {
        try {
          return JSON.parse(response.data) as T;
        } catch {
          return response.data as unknown as T;
        }
      }

      return response.data as T;
    } catch (error) {
      if (axios.isAxiosError(error)) {
        const axiosError = error as AxiosError;

        if (axiosError.response) {
          throw new EvelentError(
            axiosError.response.status,
            axiosError.response.statusText,
            axiosError.response.data,
          );
        }

        if (axiosError.request) {
          throw new EvelentError(
            0,
            "Network Error",
            `Failed to connect to ${url}: ${axiosError.message}`,
          );
        }

        throw new EvelentError(
          0,
          "Request Error",
          `Request failed: ${axiosError.message}`,
        );
      }

      throw error;
    }
  }

  async requestText(path: string): Promise<string> {
    const url = `${this.normalizeBase()}${path}`;

    try {
      const response: AxiosResponse = await axios({
        url,
        method: "GET",
        headers: this.authHeaders(),
        responseType: "text",
        validateStatus: () => true,
      });

      if (response.status < 200 || response.status >= 300) {
        throw new EvelentError(
          response.status,
          response.statusText,
          response.data,
        );
      }

      return response.data || "";
    } catch (error) {
      if (axios.isAxiosError(error)) {
        const axiosError = error as AxiosError;

        if (axiosError.response) {
          throw new EvelentError(
            axiosError.response.status,
            axiosError.response.statusText,
            axiosError.response.data,
          );
        }

        if (axiosError.request) {
          throw new EvelentError(
            0,
            "Network Error",
            `Failed to connect to ${url}: ${axiosError.message}`,
          );
        }

        throw new EvelentError(
          0,
          "Request Error",
          `Request failed: ${axiosError.message}`,
        );
      }

      throw error;
    }
  }
}

export function enc(segment: string): string {
  return encodeURIComponent(segment);
}
