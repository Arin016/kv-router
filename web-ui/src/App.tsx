import { useEffect } from "react";
import DashboardPage from "./pages/DashboardPage";
import EngineeringPage from "./pages/EngineeringPage";
import HomePage from "./pages/HomePage";
import ResearchPage from "./pages/ResearchPage";

function pathOf(): string {
  return window.location.pathname.replace(/\/+$/, "") || "/";
}

export default function App() {
  const path = pathOf();

  useEffect(() => {
    const titles: Record<string, string> = {
      "/engineering": "Architecture: what the router knows — Nostos",
      "/research": "Notes — Nostos",
      "/research/cache-affinity": "Cache affinity is not a cache guarantee — Nostos",
      "/research/queue-reservations": "A queue depth is not a reservation — Nostos",
      "/research/prompt-safe-observability": "Observability without prompts — Nostos",
      "/dashboard": "Console — Nostos",
    };
    document.title = titles[path] ?? "Nostos — Cache-aware routing for LLM inference";
  }, [path]);

  if (path === "/dashboard" || path.startsWith("/dashboard/")) return <DashboardPage />;
  if (path === "/engineering" || path.startsWith("/engineering/")) return <EngineeringPage />;
  if (path === "/research" || path.startsWith("/research/")) return <ResearchPage />;
  return <HomePage />;
}
