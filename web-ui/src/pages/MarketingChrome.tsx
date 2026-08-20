import { useEffect, type ReactNode } from "react";
import BrandMark from "../components/BrandMark";

type Props = {
  children: ReactNode;
  active?: "product" | "engineering" | "research";
};

function scrollToId(id: string) {
  document.getElementById(id)?.scrollIntoView({ behavior: "smooth", block: "start" });
}

export default function MarketingChrome({ children, active = "product" }: Props) {
  useEffect(() => {
    const runHash = () => {
      const id = window.location.hash.replace(/^#/, "");
      if (id) requestAnimationFrame(() => scrollToId(id));
    };
    runHash();
    window.addEventListener("hashchange", runHash);
    const onClick = (e: MouseEvent) => {
      const a = (e.target as Element | null)?.closest?.("a");
      if (!a) return;
      const href = a.getAttribute("href");
      if (!href) return;
      if (href.startsWith("#") && href.length > 1) {
        e.preventDefault();
        scrollToId(href.slice(1));
        history.replaceState(null, "", href);
      }
    };
    document.addEventListener("click", onClick);
    return () => {
      window.removeEventListener("hashchange", runHash);
      document.removeEventListener("click", onClick);
    };
  }, []);

  const surface = active === "product" ? "home" : "essay";

  return (
    <div className="mkt" data-surface={surface}>
      <div className="mkt-bg" aria-hidden />
      <div className="mkt-bg-grid" aria-hidden />
      <div className="mkt-bg-grain" aria-hidden />

      <header className="mkt-nav">
        <a className="mkt-brand" href="/">
          <span className="mkt-brand-mark">
            <BrandMark />
          </span>
          <span className="mkt-brand-name">Nostos</span>
        </a>
        <nav className="mkt-nav-links" aria-label="Primary">
          <a href={active === "product" ? "#arena" : "/#arena"}>Arena</a>
          <a href="/engineering" className={active === "engineering" ? "is-active" : undefined}>
            Engineering
          </a>
          <a href="/research" className={active === "research" ? "is-active" : undefined}>
            Notes
          </a>
          <a href="/dashboard" className="mkt-nav-cta">
            Console
          </a>
        </nav>
      </header>

      {children}

      <footer className="mkt-footer">
        <div className="mkt-footer-inner">
          <div>
            <a className="mkt-brand mkt-brand-sm" href="/">
              <span className="mkt-brand-mark">
                <BrandMark size={22} />
              </span>
              <span className="mkt-brand-name">Nostos</span>
            </a>
            <p className="mkt-footer-tag">
              Nostos (νόστος): homecoming. The locality layer for LLM inference — send the request
              back to the GPU that already remembers it.{" "}
              <a href="https://kv-router.vercel.app" target="_blank" rel="noreferrer">
                Live demo →
              </a>
            </p>
          </div>
          <nav className="mkt-footer-links" aria-label="Footer">
            <a href="/#arena">Arena</a>
            <a href="/engineering">Engineering</a>
            <a href="/research">Research</a>
            <a href="/dashboard">Console</a>
          </nav>
        </div>
      </footer>
    </div>
  );
}
