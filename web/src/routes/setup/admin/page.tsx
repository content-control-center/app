import {  useNavigate } from "@tanstack/react-router";

import { AppAuth } from "@/components/layout/AppAuth";
import { AuthRegisterForm } from "@/components/forms/authRegisterForm";
import { invalidateSetupComplete } from "@/services/api/setup";

function SetupAdminPage() {
  const navigate = useNavigate();

  const handleSuccess = () => {
    // First-user creation flips `setup_complete` server-side; drop the cached
    // answer so the next guard run refetches and lets the user off the
    // setup flow.
    invalidateSetupComplete();
    navigate({ to: "/auth/login" });
  };

  return (
    <AppAuth
      title="Create Administrator"
      subtitle="This account will own the instance."
      form={<AuthRegisterForm onSuccess={handleSuccess} />}
      bottomNav={
        undefined
      }
    />
  );
}

export default SetupAdminPage;
