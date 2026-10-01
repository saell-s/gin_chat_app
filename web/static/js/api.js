"use strict";
// Thin fetch wrapper around the JSON API with one-shot token refresh.
class ApiError extends Error {
    constructor(status, message) {
        super(message);
        this.name = "ApiError";
        this.status = status;
    }
}
class Api {
    constructor() {
        this.baseUrl = "/api/v1";
        this.accessToken = null;
        this.refreshToken = null;
        this.refreshing = null;
    }
    setTokens(access, refresh) {
        this.accessToken = access;
        this.refreshToken = refresh;
        if (access) {
            localStorage.setItem("access_token", access);
        }
        else {
            localStorage.removeItem("access_token");
        }
        if (refresh) {
            localStorage.setItem("refresh_token", refresh);
        }
        else {
            localStorage.removeItem("refresh_token");
        }
    }
    loadTokens() {
        this.accessToken = localStorage.getItem("access_token");
        this.refreshToken = localStorage.getItem("refresh_token");
    }
    hasSession() {
        return !!this.accessToken || !!this.refreshToken;
    }
    token() {
        return this.accessToken;
    }
    async request(method, path, body, retry = true) {
        const headers = { Accept: "application/json" };
        if (body !== undefined) {
            headers["Content-Type"] = "application/json";
        }
        if (this.accessToken) {
            headers["Authorization"] = "Bearer " + this.accessToken;
        }
        let res;
        try {
            res = await fetch(this.baseUrl + path, {
                method,
                headers,
                body: body !== undefined ? JSON.stringify(body) : undefined,
            });
        }
        catch {
            throw new ApiError(0, "Cannot reach the server");
        }
        if (res.status === 401 && retry && this.refreshToken && !path.startsWith("/auth/")) {
            const ok = await this.refreshSession();
            if (ok) {
                return this.request(method, path, body, false);
            }
            this.setTokens(null, null);
            throw new ApiError(401, "Your session expired, please sign in again");
        }
        const text = await res.text();
        let payload = null;
        if (text) {
            try {
                payload = JSON.parse(text);
            }
            catch {
                payload = null;
            }
        }
        if (!res.ok) {
            const message = payload && payload.error && payload.error.message
                ? payload.error.message
                : res.statusText || "Request failed";
            throw new ApiError(res.status, message);
        }
        return payload ? payload.data : undefined;
    }
    async refreshSession() {
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
                const payload = (await res.json());
                this.setTokens(payload.data.access_token, payload.data.refresh_token);
                return true;
            }
            catch {
                return false;
            }
            finally {
                this.refreshing = null;
            }
        })();
        return this.refreshing;
    }
    get(path) {
        return this.request("GET", path);
    }
    post(path, body) {
        return this.request("POST", path, body ?? {});
    }
    patch(path, body) {
        return this.request("PATCH", path, body ?? {});
    }
    del(path) {
        return this.request("DELETE", path);
    }
}
const api = new Api();
