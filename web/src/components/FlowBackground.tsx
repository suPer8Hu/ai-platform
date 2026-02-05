import type { ReactNode } from "react";

type FlowBackgroundProps = {
  children: ReactNode;
};

export default function FlowBackground({ children }: FlowBackgroundProps) {
  return (
    <div className="relative min-h-screen overflow-hidden bg-slate-950 text-white">
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

      <div className="relative z-10">{children}</div>
    </div>
  );
}
