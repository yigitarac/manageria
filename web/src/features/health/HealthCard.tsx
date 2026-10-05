import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { fetchHealth } from "../../api/client";
import { Button } from "../../components/ui/button";
import { cn } from "../../lib/utils";

type Status = "checking" | "ok" | "error";

export function HealthCard() {
  const { t } = useTranslation();
  const health = useQuery({
    queryKey: ["health"],
    queryFn: fetchHealth,
    retry: false,
    refetchInterval: 30_000,
  });

  const status: Status = health.isPending ? "checking" : health.isError ? "error" : "ok";

  return (
    <section className="w-full max-w-sm rounded-xl border border-border bg-card p-6 shadow-sm">
      <h2 className="text-sm font-medium text-muted-foreground">{t("health.label")}</h2>
      <p
        role="status"
        className={cn("mt-2 text-2xl font-semibold", status === "error" && "text-destructive")}
      >
        {t(`health.${status}`)}
      </p>
      {status === "error" && (
        <Button className="mt-4" size="sm" variant="outline" onClick={() => void health.refetch()}>
          {t("health.retry")}
        </Button>
      )}
    </section>
  );
}
