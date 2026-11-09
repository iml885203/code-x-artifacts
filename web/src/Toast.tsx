import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";

export interface Toast {
  message: string;
  /** An action offered with the message, like undoing what it reports. */
  action?: { label: string; run: () => void };
  /** An error stays a little longer and is announced at once. */
  error?: boolean;
}

const Context = createContext<((t: Toast) => void) | null>(null);

const SHOW_MS = 4000;
const EXIT_MS = 120;

/** A single message at the bottom of the page. A new one replaces the one showing. */
export function ToastProvider({ children }: { children: ReactNode }) {
  const [toast, setToast] = useState<(Toast & { closing: boolean; key: number }) | null>(null);
  const timers = useRef<number[]>([]);
  const counter = useRef(0);

  const clear = () => {
    timers.current.forEach((t) => {
      window.clearTimeout(t);
    });
    timers.current = [];
  };
  useEffect(() => clear, []);

  const dismiss = useCallback(() => {
    clear();
    setToast((t) => (t ? { ...t, closing: true } : t));
    timers.current.push(
      window.setTimeout(() => {
        setToast(null);
      }, EXIT_MS),
    );
  }, []);

  const show = useCallback(
    (t: Toast) => {
      clear();
      counter.current += 1;
      setToast({ ...t, closing: false, key: counter.current });
      timers.current.push(window.setTimeout(dismiss, t.error ? SHOW_MS * 2 : SHOW_MS));
    },
    [dismiss],
  );

  const value = useMemo(() => show, [show]);
  return (
    <Context value={value}>
      {children}
      <div className="toasts" aria-live="polite">
        {toast && (
          <div key={toast.key} className="toast" {...(toast.error ? { role: "alert" } : {})} data-state={toast.closing ? "closed" : "open"}>
            <span>{toast.message}</span>
            {toast.action && (
              <button
                type="button"
                className="quiet"
                onClick={() => {
                  toast.action?.run();
                  dismiss();
                }}
              >
                {toast.action.label}
              </button>
            )}
          </div>
        )}
      </div>
    </Context>
  );
}

export function useToast(): (t: Toast) => void {
  const show = useContext(Context);
  if (!show) throw new Error("useToast outside ToastProvider");
  return show;
}
