import { useMutation } from "@tanstack/react-query";

import {
  login as loginRequest,
  logout as logoutRequest,
  invalidateSession,
} from "@/services/api/sessions";
import { register as registerRequest, getMe } from "@/services/api/users";
import type { LoginPayload, Session } from "@/types/session";
import type { RegisterPayload, User } from "@/types/user";
import { useAuthStore } from "@/stores/authStore";

export function useLogin() {
  const setUser = useAuthStore((s) => s.setUser);
  return useMutation<Session, Error, LoginPayload>({
    mutationFn: loginRequest,
    onSuccess: async () => {
      invalidateSession();
      const user = await getMe();
      setUser(user);
    },
  });
}

export function useRegister() {
  const setUser = useAuthStore((s) => s.setUser);
  return useMutation<User, Error, RegisterPayload>({
    mutationFn: registerRequest,
    onSuccess: (user) => {
      setUser(user);
    },
  });
}

export function useLogout() {
  const clearUser = useAuthStore((s) => s.clearUser);
  return useMutation<void, Error, void>({
    mutationFn: logoutRequest,
    onSuccess: () => {
      invalidateSession();
      clearUser();
    },
  });
}

/**
 * Placeholder — the backend does not yet expose a "resend verification
 * email" endpoint. Wire this to the real call when it lands.
 */
export function useResendVerificationEmail() {
  return useMutation<void, Error, string>({
    mutationFn: async () => {
      throw new Error("Resend verification email is not yet implemented");
    },
  });
}
