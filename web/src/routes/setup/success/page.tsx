import { useNavigate, useLocation } from "@tanstack/react-router";

import { AppAuth } from "@/components/layout/AppAuth";
import { Button } from "@/components/ui/button";
import { ArrowUpRightIcon } from "@phosphor-icons/react";
import { useLogin } from "@/hooks/useAuth";
import { invalidateSetupComplete, markSetupComplete } from "@/services/api/setup";
import type { LoginPayload } from "@/types/session";

function SetupSuccessPage() {
  const navigate = useNavigate();
  const location = useLocation();
  const { mutate: login, isPending, error } = useLogin();

  const credentials = (location.state as { credentials?: LoginPayload }).credentials!;

  const handleLogin = () => {
    login(credentials, {
        onSuccess: async () => {
          await markSetupComplete();
          invalidateSetupComplete();
          navigate({ to: "/" });
        },
      }
    );
  };

  return (
    <AppAuth
      title="You're all set!"
      subtitle="Your administrator account has been created successfully."
      form={
        <div className="flex flex-col gap-6 shrink-0 animate-in fade-in duration-500">
          {error && (
            <p className="text-sm text-destructive">{error.message}</p>
          )}
          <Button
            type="button"
            onClick={handleLogin}
            variant="defaultInverted"
            size="default"
            className="w-full justify-between"
            loading={isPending}
            disabled={isPending}
          >
            <span>LOG IN AND PROCEED</span>
            <ArrowUpRightIcon className="size-4" />
          </Button>
        </div>
      }
      bottomNav={undefined}
    />
  );
}

export default SetupSuccessPage;
