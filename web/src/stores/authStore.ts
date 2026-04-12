import { create } from "zustand";
import { persist, devtools } from "zustand/middleware";
import { AUTH_STORE_PERSIST_KEY } from "@/stores/constants";
import { logout as logoutRequest, invalidateSession } from "@/services/api/sessions";
import type { User } from "@/types/user";

export interface AuthState {
  user: User | null;
  isAuthenticated: () => boolean;
  setUser: (user: User) => void;
  clearUser: () => void;
  logout: () => Promise<void>;
}

export const useAuthStore = create<AuthState>()(
  devtools(
    persist(
      (set, get) => ({
        user: null,
        isAuthenticated: () => get().user !== null,
        setUser: (user) => set({ user }),
        clearUser: () => set({ user: null }),
        logout: async () => {
          await logoutRequest();
          invalidateSession();
          set({ user: null });
        },
      }),
      {
        name: AUTH_STORE_PERSIST_KEY,
        partialize: (state) => ({
          user: state.user,
        }),
      }
    ),
    {
      name: AUTH_STORE_PERSIST_KEY,
    }
  )
);
