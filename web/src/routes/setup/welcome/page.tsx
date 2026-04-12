import { AppAuth } from "@/components/layout/AppAuth";
import { SetupWelcomeForm } from "@/components/forms/setupWelcomeForm";

function SetupWelcomePage() {
  return (
    <AppAuth
      title="Welcome to Ogen"
      subtitle="Let's set up your instance. This only needs to be done once."
      form={<SetupWelcomeForm />}
      bottomNav={undefined}
    />
  );
}

export default SetupWelcomePage;
