import i18n from "i18next";
import { initReactI18next } from "react-i18next";

const resources = {
  en: {
    translation: {
      "home.title": "Manageria",
      "home.tagline": "Online football management — the better manager wins.",
      "home.cta": "Open the match viewer",
      "health.label": "API",
      "health.checking": "Checking\u2026",
      "health.ok": "Healthy",
      "health.error": "Unreachable",
      "health.retry": "Retry",
      "viewer.title": "Match viewer",
      "viewer.loading": "Loading match\u2026",
      "viewer.dropHint": "Drop a match.json dump here (generate one with: make sample)",
      "viewer.badFile": "That file is not a match dump",
      "viewer.play": "Play",
      "viewer.pause": "Pause",
      "viewer.scrub": "Timeline",
      "viewer.speed": "Playback speed",
      "viewer.commentary": "Commentary",
      "viewer.stats": "Statistics",
      "viewer.patternTag": "pattern: {{id}}",
      "viewer.patternStat": "{{club}}: {{shots}} pattern shots \u00b7 {{xg}} xG",
      "stats.possession": "Possession",
      "stats.shots": "Shots",
      "stats.onTarget": "On target",
      "stats.xg": "xG",
      "stats.corners": "Corners",
      "stats.fouls": "Fouls",
      "stats.turnovers": "Turnovers",
      "stats.avgFatigue": "Fatigue",
      "event.kick_off": "Kick-off",
      "event.half_time": "Half-time",
      "event.goal": "GOAL!",
      "event.shot_saved": "Shot saved",
      "event.shot_off_target": "Shot off target",
      "event.offside": "Offside",
      "event.foul": "Foul",
      "event.yellow_card": "Yellow card",
      "event.red_card": "Red card",
      "event.injury": "Injury",
      "event.substitution": "Substitution",
      "event.error": "Costly mistake",
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
