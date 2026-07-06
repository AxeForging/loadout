import { useEffect, useRef, useState } from "react";

const reducedMotion = () =>
  typeof window !== "undefined" &&
  window.matchMedia("(prefers-reduced-motion: reduce)").matches;

/** Count from 0 to target with exponential ease-out. Instant under reduced motion. */
export function useCountUp(target: number, duration = 700): number {
  const [value, setValue] = useState(() => (reducedMotion() ? target : 0));
  const raf = useRef(0);

  useEffect(() => {
    if (reducedMotion()) {
      setValue(target);
      return;
    }
    const start = performance.now();
    const tick = (now: number) => {
      const t = Math.min(1, (now - start) / duration);
      const eased = 1 - Math.pow(2, -10 * t);
      setValue(Math.round(target * (t === 1 ? 1 : eased)));
      if (t < 1) raf.current = requestAnimationFrame(tick);
    };
    raf.current = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf.current);
  }, [target, duration]);

  return value;
}

/** Big display numeral that counts up on mount. */
export function Num({ value, className = "" }: { value: number; className?: string }) {
  const n = useCountUp(value);
  return <span className={`tnum ${className}`}>{n}</span>;
}
