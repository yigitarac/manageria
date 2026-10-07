import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { Button } from "../components/ui/button";
import { HealthCard } from "../features/health/HealthCard";

export default function HomePage() {
  const { t } = useTranslation();

  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-6 p-6">
      <div className="text-center">
        <h1 className="text-4xl font-bold tracking-tight">{t("home.title")}</h1>
        <p className="mt-2 text-muted-foreground">{t("home.tagline")}</p>
      </div>
      <HealthCard />
      <Button asChild variant="outline">
        <Link to="/viewer">{t("home.cta")}</Link>
      </Button>
    </main>
  );
}
