import { useNavigate } from "@tanstack/react-router";

import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";

export function SetupWelcomeForm() {
  const navigate = useNavigate();

  const handleProceed = () => {
    navigate({ to: "/setup/admin" });
  };

  return (
    <div className="flex flex-col gap-6 shrink-0 animate-in fade-in duration-500">
      <Button
        type="button"
        onClick={handleProceed}
        variant="defaultInverted"
        size="default"
        className="w-full justify-between"
      >
        <span>PROCEED TO SETUP</span>
        <Icon className="size-4" name="arrow_right_top" />
      </Button>
    </div>
  );
}
