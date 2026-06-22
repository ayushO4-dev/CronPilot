import { useEffect, useRef, useState } from "react";
import { api, isApiError } from "../lib/api";
import { useAuth } from "../lib/auth";
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

function fmtBytes(n: number): string {
  if (n <= 0) return "0 B";
  const u = ["B", "KB", "MB", "GB"];
  const i = Math.min(u.length - 1, Math.floor(Math.log(n) / Math.log(1024)));
  return `${(n / 1024 ** i).toFixed(i === 0 ? 0 : 1)} ${u[i]}`;
}

export function UpdatePanel() {
  const { logout } = useAuth();
  const [check, setCheck] = useState<UpdateCheck | null>(null);
  const [checking, setChecking] = useState(false);
  const [error, setError] = useState("");
  const [status, setStatus] = useState<UpdateStatus | null>(null);
  const [updating, setUpdating] = useState(false);
  const timer = useRef<number | undefined>(undefined);
  const updatingRef = useRef(false);

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
      await api.post("/api/update/apply");
      setStatus({ state: "downloading", downloaded: 0, total: 0 });
      pollStatus();
    } catch (e) {
      setError(isApiError(e) ? e.error : "update failed to start");
    }
  }

  function pollStatus() {
    const tick = async () => {
      try {
        const st = await api.get<UpdateStatus>("/api/update/status");
        setStatus(st);
        if (st.state === "error") {
          setError(st.error || "update failed");
          return;
        }
        if (st.state === "applying" || st.state === "restarting") {
          // Point of no return: log out and show the updating overlay, then
          // wait for the daemon to come back.
          if (!updatingRef.current) {
            updatingRef.current = true;
            setUpdating(true);
            void logout();
            waitForRestart();
          }
          return;
        }
      } catch {
        /* transient (e.g. server already restarting) — keep trying */
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
          window.location.reload();
          return;
        }
      } catch {
        /* server is down mid-restart */
      }
      window.setTimeout(tick, 1500);
    };
    // Give the old process time to exit before polling for the new one.
    window.setTimeout(tick, 2500);
  }

  if (updating) {
    return (
      <div
        style={{
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
        }}
      >
        <div style={{ fontSize: "var(--fs-lg)", color: "var(--fg-strong)" }}>
          Updating CronPilot…
        </div>
        <div style={{ ...muted, maxWidth: 420 }}>
          Installing {status?.latest ?? "the update"} and restarting the server.
          This page will reload automatically when it's back.
        </div>
      </div>
    );
  }

  const downloading = status?.state === "downloading";
  const pct =
    status && status.total > 0
      ? Math.round((status.downloaded / status.total) * 100)
      : 0;

  return (
    <div style={col}>
      <Button onClick={doCheck} disabled={checking || !!status}>
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
            {downloading && (
              <div>
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
                <div style={muted}>
                  downloading {fmtBytes(status!.downloaded)}
                  {status!.total > 0
                    ? ` / ${fmtBytes(status!.total)} (${pct}%)`
                    : ""}
                </div>
              </div>
            )}
            <Button variant="primary" onClick={doInstall} disabled={!!status}>
              {status ? "installing…" : `Install ${check.latest}`}
            </Button>
            <div style={muted}>
              The server will download the update, log you out, and restart.
            </div>
          </>
        ) : (
          <div style={muted}>You're on the latest version ({check.current}).</div>
        ))}
    </div>
  );
}
