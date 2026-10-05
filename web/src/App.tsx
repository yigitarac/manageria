import { useTranslation } from "react-i18next";
import { HealthCard } from "./features/health/HealthCard";

export default function App() {
  const { t } = useTranslation();

  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-6 p-6">
      <div className="text-center">
        <h1 className="text-4xl font-bold tracking-tight">{t("home.title")}</h1>
        <p className="mt-2 text-muted-foreground">{t("home.tagline")}</p>
      </div>
      <HealthCard />
    </main>
  );
}
