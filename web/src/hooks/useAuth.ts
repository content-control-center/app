import { useMutation } from "@tanstack/react-query";

import {
  login as loginRequest,
  logout as logoutRequest,
  type LoginPayload,
  type Session,
} from "@/services/api/sessions";
import {
  register as registerRequest,
  type RegisterPayload,
  type User,
} from "@/services/api/users";
import { useAuthStore } from "@/stores/authStore";

/**
 * Auth hooks — thin wrappers around the session/user services that also
 * keep the zustand `authStore` in sync so route guards (read via router
 * context) can react synchronously.
 *
 * The backend login endpoint returns a `Session`, not a `User`. Until a
 * `/api/me` endpoint exists we synthesize a minimal `User` record from the
 * session + the email the caller submitted. It's enough for
 * `isAuthenticated()` to flip; a real profile fetch can replace this later.
 */
export function useLogin() {
  const setUser = useAuthStore((s) => s.setUser);
  return useMutation<Session, Error, LoginPayload>({
    mutationFn: loginRequest,
    onSuccess: (session, variables) => {
      const now = new Date().toISOString();
      const stubUser: User = {
        id: session.user_id,
        name: "",
        email: variables.email,
        created_at: now,
        updated_at: now,
      };
      setUser(stubUser);
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
