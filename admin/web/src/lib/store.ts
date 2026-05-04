import { create } from "zustand";
import { api, setToken, clearToken, hasToken } from "./api";

interface AuthState {
  isAuthenticated: boolean;
  checkAuth: () => void;
  login: (token: string) => Promise<boolean>;
  logout: () => Promise<void>;
}

export const useAuthStore = create<AuthState>((set) => ({
  isAuthenticated: false,
  checkAuth: () => {
    set({ isAuthenticated: hasToken() });
  },
  login: async (token: string) => {
    setToken(token);
    try {
      // Use login handshake: sends Bearer, server sets HMAC cookie in response.
      // Subsequent api.get/post/del calls will auth via the cookie.
      await api.login("/auth/verify");
      set({ isAuthenticated: true });
      return true;
    } catch {
      clearToken();
      set({ isAuthenticated: false });
      return false;
    }
  },
  logout: async () => {
    try {
      await api.post("/auth/logout");
    } catch {
      // ignore errors
    }
    clearToken();
    set({ isAuthenticated: false });
  },
}));

interface ConfigState {
  config: Record<string, Record<string, string>> | null;
  loading: boolean;
  fetchConfig: () => Promise<void>;
  saveConfig: (updates: Record<string, string>) => Promise<string[]>;
}

export const useConfigStore = create<ConfigState>((set) => ({
  config: null,
  loading: false,
  fetchConfig: async () => {
    set({ loading: true });
    try {
      const data = await api.get<{ config: Record<string, Record<string, string>> }>("/config");
      set({ config: data.config });
    } finally {
      set({ loading: false });
    }
  },
  saveConfig: async (updates: Record<string, string>) => {
    const data = await api.post<{ updated: string[] }>("/config", updates);
    return data.updated;
  },
}));
