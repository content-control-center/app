import { useNavigate } from "@tanstack/react-router";

import { AppAuth } from "@/components/layout/AppAuth";
import { AuthRegisterForm } from "@/components/forms/authRegisterForm";
import type { LoginPayload } from "@/services/api/sessions";

function SetupAdminPage() {
  const navigate = useNavigate();

  const handleSuccess = (credentials: LoginPayload) => {
    navigate({
      to: "/setup/success",
      state: (prev) => ({ ...prev, credentials }),
    });
  };

  return (
    <AppAuth
      title="Create Administrator"
      subtitle="This account will own the instance."
      form={<AuthRegisterForm onSuccess={handleSuccess} />}
      bottomNav={undefined}
    />
  );
}

export default SetupAdminPage;
