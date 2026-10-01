// Thin fetch wrapper around the JSON API with one-shot token refresh.

class ApiError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

class Api {
  private baseUrl = "/api/v1";
  private accessToken: string | null = null;
  private refreshToken: string | null = null;
  private refreshing: Promise<boolean> | null = null;

  setTokens(access: string | null, refresh: string | null): void {
    this.accessToken = access;
    this.refreshToken = refresh;
    if (access) {
      localStorage.setItem("access_token", access);
    } else {
      localStorage.removeItem("access_token");
    }
    if (refresh) {
      localStorage.setItem("refresh_token", refresh);
    } else {
      localStorage.removeItem("refresh_token");
    }
  }

  loadTokens(): void {
    this.accessToken = localStorage.getItem("access_token");
    this.refreshToken = localStorage.getItem("refresh_token");
  }

  hasSession(): boolean {
    return !!this.accessToken || !!this.refreshToken;
  }

  token(): string | null {
    return this.accessToken;
  }

  async request<T>(method: string, path: string, body?: unknown, retry = true): Promise<T> {
    const headers: Record<string, string> = { Accept: "application/json" };
    if (body !== undefined) {
      headers["Content-Type"] = "application/json";
    }
    if (this.accessToken) {
      headers["Authorization"] = "Bearer " + this.accessToken;
    }

    let res: Response;
    try {
      res = await fetch(this.baseUrl + path, {
        method,
        headers,
        body: body !== undefined ? JSON.stringify(body) : undefined,
      });
    } catch {
      throw new ApiError(0, "Cannot reach the server");
    }

    if (res.status === 401 && retry && this.refreshToken && !path.startsWith("/auth/")) {
      const ok = await this.refreshSession();
      if (ok) {
        return this.request<T>(method, path, body, false);
      }
      this.setTokens(null, null);
      throw new ApiError(401, "Your session expired, please sign in again");
    }

    const text = await res.text();
    let payload: any = null;
    if (text) {
      try {
        payload = JSON.parse(text);
      } catch {
        payload = null;
      }
    }

    if (!res.ok) {
      const message =
        payload && payload.error && payload.error.message
          ? payload.error.message
          : res.statusText || "Request failed";
      throw new ApiError(res.status, message);
    }
    return payload ? (payload.data as T) : (undefined as unknown as T);
  }

  private async refreshSession(): Promise<boolean> {
    if (this.refreshing) {
      return this.refreshing;
    }
    const refresh = this.refreshToken;
    if (!refresh) {
      return false;
    }
    this.refreshing = (async () => {
      try {
        const res = await fetch(this.baseUrl + "/auth/refresh", {
          method: "POST",
          headers: { "Content-Type": "application/json", Accept: "application/json" },
          body: JSON.stringify({ refresh_token: refresh }),
        });
        if (!res.ok) {
          return false;
        }
        const payload = (await res.json()) as Envelope<TokenPair>;
        this.setTokens(payload.data.access_token, payload.data.refresh_token);
        return true;
      } catch {
        return false;
      } finally {
        this.refreshing = null;
      }
    })();
    return this.refreshing;
  }

  get<T>(path: string): Promise<T> {
    return this.request<T>("GET", path);
  }

  post<T>(path: string, body?: unknown): Promise<T> {
    return this.request<T>("POST", path, body ?? {});
  }

  patch<T>(path: string, body?: unknown): Promise<T> {
    return this.request<T>("PATCH", path, body ?? {});
  }

  del<T>(path: string): Promise<T> {
    return this.request<T>("DELETE", path);
  }
}

const api = new Api();
