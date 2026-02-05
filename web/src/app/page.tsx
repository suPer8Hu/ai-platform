import Link from "next/link";
import { DM_Serif_Display, Space_Grotesk } from "next/font/google";

const display = DM_Serif_Display({ subsets: ["latin"], weight: "400" });
const body = Space_Grotesk({ subsets: ["latin"], weight: ["400", "500", "700"] });

export default function Home() {
  return (
    <div className={`${body.className} relative min-h-screen overflow-hidden bg-slate-950 text-white`}>
      <style>{`
        @keyframes floatOne {
          0% { transform: translate3d(0, 0, 0) scale(1); }
          50% { transform: translate3d(20px, -30px, 0) scale(1.04); }
          100% { transform: translate3d(0, 0, 0) scale(1); }
        }
        @keyframes floatTwo {
          0% { transform: translate3d(0, 0, 0) scale(1); }
          50% { transform: translate3d(-30px, 20px, 0) scale(0.98); }
          100% { transform: translate3d(0, 0, 0) scale(1); }
        }
        @keyframes floatThree {
          0% { transform: translate3d(0, 0, 0) scale(1); }
          50% { transform: translate3d(10px, 25px, 0) scale(1.06); }
          100% { transform: translate3d(0, 0, 0) scale(1); }
        }
        @keyframes fadeUp {
          0% { opacity: 0; transform: translate3d(0, 12px, 0); }
          100% { opacity: 1; transform: translate3d(0, 0, 0); }
        }
      `}</style>

      <div className="pointer-events-none absolute inset-0">
        <div className="absolute inset-0 bg-[radial-gradient(ellipse_at_top,_rgba(16,185,129,0.16),_transparent_60%)]" />
        <div className="absolute inset-0 bg-[radial-gradient(ellipse_at_bottom,_rgba(245,158,11,0.12),_transparent_55%)]" />
        <div
          className="absolute -top-40 left-1/2 h-[28rem] w-[28rem] -translate-x-1/2 rounded-full bg-emerald-400/25 blur-3xl"
          style={{ animation: "floatOne 18s ease-in-out infinite" }}
        />
        <div
          className="absolute bottom-[-10rem] left-[-6rem] h-[22rem] w-[22rem] rounded-full bg-amber-300/20 blur-3xl"
          style={{ animation: "floatTwo 22s ease-in-out infinite" }}
        />
        <div
          className="absolute top-12 right-[-6rem] h-[24rem] w-[24rem] rounded-full bg-teal-300/15 blur-3xl"
          style={{ animation: "floatThree 20s ease-in-out infinite" }}
        />
        <div className="absolute inset-0 bg-[linear-gradient(120deg,_rgba(255,255,255,0.05),_transparent_40%,_rgba(255,255,255,0.04))]" />
      </div>

      <header className="relative z-10 flex items-center justify-between px-6 pt-6 sm:px-10">
        <div className="text-xs font-medium uppercase tracking-[0.35em] text-emerald-100/70">
          Gopherchat
        </div>
        <Link
          className="rounded-full border border-white/20 px-4 py-1.5 text-sm text-emerald-50/90 transition hover:border-white/50 hover:text-white"
          href="/login"
        >
          Login
        </Link>
      </header>

      <main className="relative z-10 flex min-h-[calc(100vh-96px)] items-center justify-center px-6 sm:px-10">
        <div className="text-center">
          <div
            className={`${display.className} text-5xl tracking-tight sm:text-7xl`}
            style={{ animation: "fadeUp 700ms ease-out both" }}
          >
            Gopherchat
          </div>
          <p
            className="mx-auto mt-4 max-w-md text-sm text-emerald-50/70 sm:text-base"
            style={{ animation: "fadeUp 900ms ease-out both" }}
          >
            A fluid space for focused AI conversations.
          </p>
          <div className="mt-8" style={{ animation: "fadeUp 1100ms ease-out both" }}>
            <Link
              className="inline-flex items-center justify-center rounded-full bg-gradient-to-r from-emerald-300 via-teal-200 to-amber-200 px-6 py-3 text-sm font-semibold text-slate-900 shadow-lg shadow-emerald-400/20 transition hover:scale-[1.02] hover:shadow-emerald-300/40"
              href="/login"
            >
              Start a chat
            </Link>
          </div>
        </div>
      </main>
    </div>
  );
}
