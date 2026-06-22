import { useEffect, useRef, useState } from "react";
import { api, isApiError } from "../lib/api";
import { Button } from "./ui";
import type { UpdateCheck, UpdateStatus } from "../lib/types";

const muted = { color: "var(--muted)", fontSize: "var(--fs-sm)" } as const;
const errStyle = { color: "var(--err)", fontSize: "var(--fs-sm)" } as const;
const col = {
  display: "flex",
  flexDirection: "column",
  gap: "var(--space-3)",
  maxWidth: 440,
} as const;
const overlay = {
  position: "fixed",
  inset: 0,
  zIndex: 1000,
  background: "var(--bg)",
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  gap: "var(--space-3)",
  textAlign: "center",
  padding: "var(--space-4)",
} as const;

function fmtBytes(n: number): string {
  if (n <= 0) return "0 B";
  const u = ["B", "KB", "MB", "GB"];
  const i = Math.min(u.length - 1, Math.floor(Math.log(n) / Math.log(1024)));
  return `${(n / 1024 ** i).toFixed(i === 0 ? 0 : 1)} ${u[i]}`;
}

export function UpdatePanel() {
  const [check, setCheck] = useState<UpdateCheck | null>(null);
  const [checking, setChecking] = useState(false);
  const [error, setError] = useState("");
  const [status, setStatus] = useState<UpdateStatus | null>(null);
  const [installing, setInstalling] = useState(false);
  const timer = useRef<number | undefined>(undefined);
  const restartingRef = useRef(false);

  useEffect(
    () => () => {
      if (timer.current) window.clearTimeout(timer.current);
    },
    [],
  );

  async function doCheck() {
    setChecking(true);
    setError("");
    try {
      setCheck(await api.get<UpdateCheck>("/api/update/check"));
    } catch (e) {
      setError(isApiError(e) ? e.error : "could not check for updates");
    } finally {
      setChecking(false);
    }
  }

  async function doInstall() {
    setError("");
    try {
      await api.post("/api/update/apply"); // starts the server-side download
    } catch (e) {
      setError(isApiError(e) ? e.error : "update failed to start");
      return;
    }
    // Switch to the full-screen updating screen and end the session right away.
    // We log out via the API (clearing the cookie) rather than the auth context
    // so this component stays mounted to drive the rest of the flow.
    setInstalling(true);
    setStatus({ state: "downloading", downloaded: 0, total: 0 });
    void api.post("/api/auth/logout").catch(() => {});
    pollStatus();
  }

  function pollStatus() {
    const tick = async () => {
      try {
        const st = await api.get<UpdateStatus>("/api/update/status");
        setStatus(st);
        if (st.state === "error") {
          setError(st.error || "update failed");
          return; // overlay shows the error
        }
        if (st.state === "applying" || st.state === "restarting") {
          if (!restartingRef.current) {
            restartingRef.current = true;
            waitForRestart();
          }
          return; // stop polling status; wait for the server to come back
        }
      } catch {
        /* transient — the server may already be restarting */
      }
      timer.current = window.setTimeout(tick, 700);
    };
    void tick();
  }

  function waitForRestart() {
    const tick = async () => {
      try {
        const r = await fetch("/api/health", { cache: "no-store" });
        if (r.ok) {
          window.location.reload(); // back online → reload to the login screen
          return;
        }
      } catch {
        /* server is down mid-restart */
      }
      window.setTimeout(tick, 1500);
    };
    window.setTimeout(tick, 2500); // let the old process exit first
  }

  if (installing) {
    const st = status?.state ?? "downloading";
    const pct =
      status && status.total > 0
        ? Math.round((status.downloaded / status.total) * 100)
        : 0;
    return (
      <div style={overlay}>
        {error ? (
          <>
            <div style={{ fontSize: "var(--fs-lg)", color: "var(--err)" }}>
              Update failed
            </div>
            <div style={{ ...muted, maxWidth: 420 }}>{error}</div>
            <Button onClick={() => window.location.reload()}>Back to login</Button>
          </>
        ) : (
          <>
            <div style={{ fontSize: "var(--fs-lg)", color: "var(--fg-strong)" }}>
              {st === "downloading"
                ? "Downloading update…"
                : st === "applying"
                  ? "Installing update…"
                  : "Restarting server…"}
            </div>
            {st === "downloading" && status && (
              <div style={{ width: 280 }}>
                <div
                  style={{
                    height: 6,
                    background: "var(--bg-inset)",
                    border: "1px solid var(--border)",
                  }}
                >
                  <div
                    style={{
                      height: "100%",
                      width: `${pct}%`,
                      background: "var(--accent)",
                      transition: "width 0.2s",
                    }}
                  />
                </div>
                <div style={{ ...muted, marginTop: "var(--space-2)" }}>
                  {fmtBytes(status.downloaded)}
                  {status.total > 0 ? ` / ${fmtBytes(status.total)} (${pct}%)` : ""}
                </div>
              </div>
            )}
            <div style={{ ...muted, maxWidth: 420 }}>
              You've been signed out. This page reloads automatically once{" "}
              {status?.latest ?? "the update"} is live.
            </div>
          </>
        )}
      </div>
    );
  }

  return (
    <div style={col}>
      <Button onClick={doCheck} disabled={checking}>
        {checking ? "checking…" : "Check for updates"}
      </Button>

      {error && <div style={errStyle}>{error}</div>}

      {check &&
        (check.available ? (
          <>
            <div>
              Update available:{" "}
              <strong>
                {check.current} → {check.latest}
              </strong>
            </div>
            {check.notes && (
              <pre
                style={{
                  ...muted,
                  whiteSpace: "pre-wrap",
                  maxHeight: 160,
                  overflow: "auto",
                  margin: 0,
                  border: "1px solid var(--border)",
                  padding: "var(--space-2)",
                }}
              >
                {check.notes}
              </pre>
            )}
            <Button variant="primary" onClick={doInstall}>
              Install {check.latest}
            </Button>
            <div style={muted}>
              The server downloads the update, signs you out, and restarts on the
              new version.
            </div>
          </>
        ) : (
          <div style={muted}>You're on the latest version ({check.current}).</div>
        ))}
    </div>
  );
}
