import i18n from "i18next";
import { initReactI18next } from "react-i18next";

const resources = {
  en: {
    translation: {
      "home.title": "Manageria",
      "home.tagline": "Online football management — the better manager wins.",
      "health.label": "API",
      "health.checking": "Checking\u2026",
      "health.ok": "Healthy",
      "health.error": "Unreachable",
      "health.retry": "Retry",
    },
  },
} as const;

void i18n.use(initReactI18next).init({
  resources,
  lng: "en",
  fallbackLng: "en",
  interpolation: { escapeValue: false },
});

export default i18n;